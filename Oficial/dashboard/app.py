import os
import pandas as pd
import plotly.express as px
import plotly.graph_objects as go
from dash import Dash, dcc, html, Input, Output
import json

import sqlalchemy
import pymongo

# ── Rutas y Conexión ──────────────────────────────────────────────────────────
def get_data_from_db():
    host = os.environ.get("DB_HOST", "localhost")
    user = os.environ.get("DB_USER", "postgres")
    password = os.environ.get("DB_PASSWORD", "adminpassword")
    dbname = os.environ.get("DB_NAME", "sistema_electoral")
    port = os.environ.get("DB_PORT", "5432")

    engine = sqlalchemy.create_engine(f"postgresql://{user}:{password}@{host}:{port}/{dbname}")

    query = """
    SELECT
        dt.departamento AS "Departamento",
        dt.municipio AS "Municipio",
        COALESCE(SUM(m.votantes_habilitados), 0) AS "VotantesHabilitados",
        COALESCE(SUM(pap.votos_validos), 0) AS "VotosValidos",
        COALESCE(SUM(pap.votos_blancos), 0) AS "VotosBlancos",
        COALESCE(SUM(pap.votos_nulos), 0) AS "VotosNulos",
        COALESCE(SUM(v.P1), 0) AS "P1",
        COALESCE(SUM(v.P2), 0) AS "P2",
        COALESCE(SUM(v.P3), 0) AS "P3",
        COALESCE(SUM(v.P4), 0) AS "P4"
    FROM
        distribucion_territorial dt
        JOIN recinto_electoral re ON dt.codigo_territorial = re.codigo_territorial
        JOIN mesas m ON re.codigo_recinto = m.codigo_recinto
        JOIN papeleta pap ON m.codigo_acta = pap.codigo_acta
        LEFT JOIN (
            SELECT
                id_papeleta,
                SUM(CASE WHEN id_partido = 1 THEN cantidad_votos ELSE 0 END) AS P1,
                SUM(CASE WHEN id_partido = 2 THEN cantidad_votos ELSE 0 END) AS P2,
                SUM(CASE WHEN id_partido = 3 THEN cantidad_votos ELSE 0 END) AS P3,
                SUM(CASE WHEN id_partido = 4 THEN cantidad_votos ELSE 0 END) AS P4
            FROM detalle_votos_partido
            GROUP BY id_papeleta
        ) v ON pap.id_papeleta = v.id_papeleta
    GROUP BY
        dt.departamento, dt.municipio
    """
    try:
        muni_df = pd.read_sql(query, engine)
        if muni_df.empty:
            muni_df = pd.DataFrame(columns=["Departamento", "Municipio", "VotantesHabilitados", "VotosValidos", "VotosBlancos", "VotosNulos", "P1", "P2", "P3", "P4"])
            muni_df.loc[0] = ["Sin Datos", "Sin Datos", 1, 0, 0, 0, 0, 0, 0, 0]
        # Aggregate to department level
        dept_df = muni_df.groupby("Departamento", as_index=False).sum(numeric_only=True)
        return dept_df, muni_df
    except Exception as e:
        print(f"Error reading from DB: {e}")
        muni_df = pd.DataFrame(columns=["Departamento", "Municipio", "VotantesHabilitados", "VotosValidos", "VotosBlancos", "VotosNulos", "P1", "P2", "P3", "P4"])
        muni_df.loc[0] = ["Error BD", "Error BD", 1, 0, 0, 0, 0, 0, 0, 0]
        dept_df = muni_df.groupby("Departamento", as_index=False).sum(numeric_only=True)
        return dept_df, muni_df

# Nombre de columnas de votos por partido
PARTY_COLS = ["P1", "P2", "P3", "P4"]
PARTY_NAMES = {
    "P1": "Partido 1 (MAS-IPSP)",
    "P2": "Partido 2 (CC)",
    "P3": "Partido 3 (FPV)",
    "P4": "Partido 4 (APB)",
}
PARTY_COLORS = {
    "P1": "#e63946",   # rojo
    "P2": "#457b9d",   # azul
    "P3": "#2a9d8f",   # verde teal
    "P4": "#f4a261",   # naranja
}

num_cols = PARTY_COLS + ["VotosValidos", "VotosBlancos", "VotosNulos", "VotantesHabilitados"]

def get_processed_data():
    dept_df, muni_df = get_data_from_db()
    dept_df, total, total_votos = process_df(dept_df)
    muni_df, _, _ = process_df(muni_df)
    return dept_df, muni_df, total, total_votos


def process_df(df):
    for c in num_cols:
        df[c] = pd.to_numeric(df[c], errors="coerce").fillna(0)
    df["Participacion"] = (df["VotantesHabilitados"].replace(0, pd.NA)
                           .pipe(lambda s: df["VotosValidos"] / s * 100)).round(1).fillna(0)
    df["Ganador"] = df[PARTY_COLS].idxmax(axis=1).map(PARTY_NAMES)
    total = df[PARTY_COLS + ["VotosValidos","VotosBlancos","VotosNulos","VotantesHabilitados"]].sum()
    total_votos = total["VotosValidos"]
    return df, total, total_votos


def get_mongo_data():
    try:
        client = pymongo.MongoClient("mongodb+srv://luxxogc_db_user:Mongo@cluster0.wftdss1.mongodb.net/?appName=Cluster0")
        db = client["electoral_rrv"]
        collection = db["Transcripciones"]
        
        # Fetch all documents
        docs = list(collection.find({}, {"_id": 0}))  # Exclude _id
        
        if not docs:
            # Empty data
            mongo_df = pd.DataFrame(columns=["Departamento", "Municipio", "VotantesHabilitados", "VotosValidos", "VotosBlancos", "VotosNulos", "P1", "P2", "P3", "P4"])
            mongo_df.loc[0] = ["Sin Datos", "Sin Datos", 1, 0, 0, 0, 0, 0, 0, 0]
        else:
            mongo_df = pd.DataFrame(docs)
            # Rename c1 to P1, etc.
            rename_map = {"c1": "P1", "c2": "P2", "c3": "P3", "c4": "P4"}
            mongo_df.rename(columns=rename_map, inplace=True)
            # Ensure all required columns exist
            required_cols = ["Departamento", "Municipio", "VotantesHabilitados", "VotosValidos", "VotosBlancos", "VotosNulos", "P1", "P2", "P3", "P4"]
            for col in required_cols:
                if col not in mongo_df.columns:
                    mongo_df[col] = 0
        
        # Group by Departamento and Municipio for muni_df
        mongo_muni_df = mongo_df.groupby(["Departamento", "Municipio"], as_index=False).sum(numeric_only=True)
        # Aggregate to department level
        mongo_dept_df = mongo_muni_df.groupby("Departamento", as_index=False).sum(numeric_only=True)
        
        return mongo_dept_df, mongo_muni_df
    except Exception as e:
        print(f"Error reading from MongoDB: {e}")
        mongo_muni_df = pd.DataFrame(columns=["Departamento", "Municipio", "VotantesHabilitados", "VotosValidos", "VotosBlancos", "VotosNulos", "P1", "P2", "P3", "P4"])
        mongo_muni_df.loc[0] = ["Error Mongo", "Error Mongo", 1, 0, 0, 0, 0, 0, 0, 0]
        mongo_dept_df = mongo_muni_df.groupby("Departamento", as_index=False).sum(numeric_only=True)
        return mongo_dept_df, mongo_muni_df


# ── GeoJSON Bolivia departamentos ────────────────────────────────────────────
GEOJSON_URL = "https://raw.githubusercontent.com/juanchos2018/bolivia-geojson/main/bolivia-departamentos.json"

DEPT_MAP = {
    "Beni":        "El Beni",
    "Chuquisaca":  "Chuquisaca",
    "Cochabamba":  "Cochabamba",
    "La Paz":      "La Paz",
    "Oruro":       "Oruro",
    "Pando":       "Pando",
    "Potosí":      "Potosí",
    "Santa Cruz":  "Santa Cruz",
    "Tarija":      "Tarija",
}

# ── App ──────────────────────────────────────────────────────────────────────
app = Dash(
    __name__,
    title="Dashboard Electoral Bolivia",
    suppress_callback_exceptions=True,
    meta_tags=[{"name": "viewport", "content": "width=device-width, initial-scale=1"}],
)
server = app.server

# ── Helpers de gráficas ──────────────────────────────────────────────────────

def make_bar_national(total, total_votos):
    vals = [total[p] for p in PARTY_COLS]
    # Evitar división por cero
    pct = [(v/total_votos*100) if total_votos > 0 else 0 for v in vals]
    fig = go.Figure(go.Bar(
        x=[PARTY_NAMES[p] for p in PARTY_COLS],
        y=vals,
        marker_color=list(PARTY_COLORS.values()),
        text=[f"{v:,.0f}<br>({p:.1f}%)" for v, p in zip(vals, pct)],
        textposition="outside",
        hovertemplate="%{x}<br>Votos: %{y:,.0f}<extra></extra>",
    ))
    fig.update_layout(**_layout("Votos totales por Partido (Nacional)"))
    return fig


def make_donut(total):
    vals = [total[p] for p in PARTY_COLS]
    labels = [PARTY_NAMES[p] for p in PARTY_COLS]
    fig = go.Figure(go.Pie(
        labels=labels, values=vals,
        hole=0.55,
        marker_colors=list(PARTY_COLORS.values()),
        textinfo="label+percent",
        hovertemplate="%{label}<br>%{value:,.0f} votos<extra></extra>",
    ))
    fig.update_layout(**_layout("Distribución porcentual"))
    return fig


def make_dept_bar(dept_df):
    melted = dept_df.melt(id_vars="Departamento", value_vars=PARTY_COLS,
                           var_name="Partido", value_name="Votos")
    melted["PartidoLabel"] = melted["Partido"].map(PARTY_NAMES)
    fig = px.bar(
        melted, x="Departamento", y="Votos", color="Partido",
        color_discrete_map=PARTY_COLORS,
        barmode="group",
        labels={"Departamento": "Departamento", "Votos": "Votos", "Partido": "Partido"},
        custom_data=["PartidoLabel"],
    )
    fig.update_traces(hovertemplate="%{customdata[0]}<br>%{y:,.0f} votos<extra></extra>")
    fig.update_layout(**_layout("Votos por Departamento y Partido"))
    return fig


def make_heatmap_party(party, dept_df):
    sub = dept_df[["Departamento", party]].copy()
    sub["DeptGeo"] = sub["Departamento"].map(DEPT_MAP)
    fig = px.choropleth(
        sub,
        geojson=GEOJSON_URL,
        locations="DeptGeo",
        featureidkey="properties.NOMBRE_DEP",
        color=party,
        color_continuous_scale=[[0, "#f5f5f5"], [0.5, PARTY_COLORS[party] + "99"], [1, PARTY_COLORS[party]]],
        hover_name="Departamento",
        hover_data={party: ":,.0f", "DeptGeo": False},
        labels={party: "Votos"},
        fitbounds="locations",
        basemap_visible=False,
    )
    fig.update_layout(**_layout(f"Mapa de calor — {PARTY_NAMES[party]}"))
    return fig


def make_valid_blank_null(dept_df):
    cats = ["VotosValidos", "VotosBlancos", "VotosNulos"]
    colors = ["#2a9d8f", "#adb5bd", "#e63946"]
    fig = go.Figure()
    for cat, col in zip(cats, colors):
        fig.add_trace(go.Bar(
            name=cat.replace("Votos", ""),
            x=dept_df["Departamento"],
            y=dept_df[cat],
            marker_color=col,
            hovertemplate=f"{cat}: %{{y:,.0f}}<extra></extra>",
        ))
    fig.update_layout(barmode="stack", **_layout("Válidos / Blancos / Nulos por Departamento"))
    return fig


def make_participation(dept_df):
    sorted_df = dept_df.sort_values("Participacion", ascending=True)
    fig = go.Figure(go.Bar(
        x=sorted_df["Participacion"],
        y=sorted_df["Departamento"],
        orientation="h",
        marker_color="#457b9d",
        text=[f"{v:.1f}%" for v in sorted_df["Participacion"]],
        textposition="outside",
        hovertemplate="%{y}: %{x:.1f}%<extra></extra>",
    ))
    fig.update_layout(**_layout("Tasa de participación por Departamento (%)"))
    return fig


def make_comparison_bar(total_pg, total_mongo, total_votos_pg, total_votos_mongo):
    categories = ["VotosValidos", "VotosBlancos", "VotosNulos"] + PARTY_COLS
    labels = ["Válidos", "Blancos", "Nulos"] + [PARTY_NAMES[p] for p in PARTY_COLS]
    
    pg_vals = [total_pg[cat] for cat in categories]
    mongo_vals = [total_mongo[cat] for cat in categories]
    
    fig = go.Figure()
    fig.add_trace(go.Bar(
        name="PostgreSQL",
        x=labels,
        y=pg_vals,
        marker_color="#457b9d",
        offsetgroup=0,
    ))
    fig.add_trace(go.Bar(
        name="MongoDB",
        x=labels,
        y=mongo_vals,
        marker_color="#e63946",
        offsetgroup=1,
    ))
    fig.update_layout(
        **_layout("Comparación Nacional: PostgreSQL vs MongoDB"),
        barmode="group",
        xaxis_title="Categoría",
        yaxis_title="Votos",
    )
    return fig


def make_comparison_table(total_pg, total_mongo):
    rows = []
    categories = ["VotosValidos", "VotosBlancos", "VotosNulos"] + PARTY_COLS
    labels = ["Votos Válidos", "Votos Blancos", "Votos Nulos"] + [PARTY_NAMES[p] for p in PARTY_COLS]
    
    for label, cat in zip(labels, categories):
        pg_val = int(total_pg[cat])
        mongo_val = int(total_mongo[cat])
        diff = pg_val - mongo_val
        diff_str = f"{diff:+,}" if diff != 0 else "0"
        rows.append(html.Tr([
            html.Td(label),
            html.Td(f"{pg_val:,}"),
            html.Td(f"{mongo_val:,}"),
            html.Td(diff_str, style={"color": "#e63946" if diff < 0 else "#2a9d8f" if diff > 0 else "#cbd5e1"}),
        ]))
    
    header = html.Tr([
        html.Th("Categoría"),
        html.Th("PostgreSQL"),
        html.Th("MongoDB"),
        html.Th("Diferencia"),
    ])
    return html.Table([html.Thead(header), html.Tbody(rows)], className="comparison-table")


def _layout(title):
    return dict(
        title=dict(text=title, x=0.5, xanchor="center", font=dict(size=15, color="#e2e8f0")),
        paper_bgcolor="#1e293b",
        plot_bgcolor="#1e293b",
        font=dict(color="#cbd5e1", family="Inter, sans-serif"),
        margin=dict(l=30, r=30, t=55, b=40),
        legend=dict(bgcolor="rgba(0,0,0,0)", font=dict(size=11)),
        xaxis=dict(showgrid=False, color="#94a3b8", tickfont=dict(size=11)),
        yaxis=dict(showgrid=True, gridcolor="#334155", color="#94a3b8", tickfont=dict(size=11)),
        coloraxis_colorbar=dict(tickfont=dict(color="#cbd5e1"), title=dict(font=dict(color="#cbd5e1"))),
    )


def kpi(label, value, sub=""):
    return html.Div([
        html.P(label, className="kpi-label"),
        html.H3(value, className="kpi-value"),
        html.P(sub, className="kpi-sub") if sub else None,
    ], className="kpi-card")


def serve_layout():
    dept_df, muni_df, total, total_votos = get_processed_data()
    mongo_dept_df, mongo_muni_df = get_mongo_data()
    mongo_dept_df, mongo_total, mongo_total_votos = process_df(mongo_dept_df)
    mongo_muni_df, _, _ = process_df(mongo_muni_df)
    ganador_nac = PARTY_NAMES[dept_df[PARTY_COLS].sum().idxmax()]
    mongo_ganador_nac = PARTY_NAMES[mongo_dept_df[PARTY_COLS].sum().idxmax()] if not mongo_dept_df.empty else "N/A"

    kpis = html.Div([
        kpi("Total Votos Válidos", f"{int(total_votos):,}"),
        kpi("Total Votos Blancos", f"{int(total['VotosBlancos']):,}"),
        kpi("Total Votos Nulos", f"{int(total['VotosNulos']):,}"),
        kpi("Habilitados", f"{int(total['VotantesHabilitados']):,}"),
        kpi("Ganador Nacional", ganador_nac),
    ], className="kpi-row")

    return html.Div([
        # Header
        html.Header([
            html.Div([
                html.H1("🗳️ Dashboard Electoral — Bolivia", className="header-title"),
                html.P("Sistema de Conteo de Votos RRV · Análisis Comparativo PostgreSQL vs MongoDB",
                       className="header-sub"),
            ], className="header-inner"),
        ], className="site-header"),

        # Main content
        html.Main([
            # PostgreSQL Dashboard Section
            html.Section([
                html.H2("📊 Dashboard PostgreSQL", className="section-title"),
                kpis,

                # Fila 1: Nacional bar + donut
                html.Div([
                    html.Div(dcc.Graph(figure=make_bar_national(total, total_votos), config={"displayModeBar": False},
                                       style={"height": "380px"}), className="card col-2"),
                    html.Div(dcc.Graph(figure=make_donut(total), config={"displayModeBar": False},
                                       style={"height": "380px"}), className="card col-1"),
                ], className="row"),

                # Fila 2: Por departamento agrupado
                html.Div([
                    html.Div(dcc.Graph(figure=make_dept_bar(dept_df), config={"displayModeBar": False},
                                       style={"height": "400px"}), className="card col-full"),
                ], className="row"),

                # Fila 3: Válidos/Blancos/Nulos + Participación
                html.Div([
                    html.Div(dcc.Graph(figure=make_valid_blank_null(dept_df), config={"displayModeBar": False},
                                       style={"height": "380px"}), className="card col-2"),
                    html.Div(dcc.Graph(figure=make_participation(dept_df), config={"displayModeBar": False},
                                       style={"height": "380px"}), className="card col-1"),
                ], className="row"),

                # Sección mapas de calor
                html.Div([
                    html.H3("🗺️ Mapas de Calor por Partido", className="section-title"),
                    html.P("Selecciona un partido para visualizar su distribución en el mapa de Bolivia.",
                           className="section-sub"),
                    dcc.Tabs(id="party-tabs", value="P1", children=[
                        dcc.Tab(label=PARTY_NAMES[p], value=p,
                                className="custom-tab", selected_className="custom-tab--selected")
                        for p in PARTY_COLS
                    ], className="tabs-container"),
                    html.Div(id="heatmap-container"),
                ], className="card col-full"),

                # Tabla resumen departamental
                html.Div([
                    html.H3("📊 Resumen por Municipio", className="section-title"),
                    html.Div(id="muni-table"),
                ], className="card col-full"),

            ], className="dashboard-section"),

            # MongoDB Dashboard Section
            html.Section([
                html.H2("📊 Dashboard MongoDB", className="section-title"),
                html.Div([
                    kpi("Total Votos Válidos", f"{int(mongo_total_votos):,}"),
                    kpi("Total Votos Blancos", f"{int(mongo_total['VotosBlancos']):,}"),
                    kpi("Total Votos Nulos", f"{int(mongo_total['VotosNulos']):,}"),
                    kpi("Habilitados", f"{int(mongo_total['VotantesHabilitados']):,}"),
                    kpi("Ganador Nacional", mongo_ganador_nac),
                ], className="kpi-row"),

                # Fila 1: Nacional bar + donut
                html.Div([
                    html.Div(dcc.Graph(figure=make_bar_national(mongo_total, mongo_total_votos), config={"displayModeBar": False},
                                       style={"height": "380px"}), className="card col-2"),
                    html.Div(dcc.Graph(figure=make_donut(mongo_total), config={"displayModeBar": False},
                                       style={"height": "380px"}), className="card col-1"),
                ], className="row"),

                # Fila 2: Por departamento agrupado
                html.Div([
                    html.Div(dcc.Graph(figure=make_dept_bar(mongo_dept_df), config={"displayModeBar": False},
                                       style={"height": "400px"}), className="card col-full"),
                ], className="row"),

                # Fila 3: Válidos/Blancos/Nulos + Participación
                html.Div([
                    html.Div(dcc.Graph(figure=make_valid_blank_null(mongo_dept_df), config={"displayModeBar": False},
                                       style={"height": "380px"}), className="card col-2"),
                    html.Div(dcc.Graph(figure=make_participation(mongo_dept_df), config={"displayModeBar": False},
                                       style={"height": "380px"}), className="card col-1"),
                ], className="row"),

                # Sección mapas de calor
                html.Div([
                    html.H3("🗺️ Mapas de Calor por Partido", className="section-title"),
                    html.P("Selecciona un partido para visualizar su distribución en el mapa de Bolivia.",
                           className="section-sub"),
                    dcc.Tabs(id="mongo-party-tabs", value="P1", children=[
                        dcc.Tab(label=PARTY_NAMES[p], value=p,
                                className="custom-tab", selected_className="custom-tab--selected")
                        for p in PARTY_COLS
                    ], className="tabs-container"),
                    html.Div(id="mongo-heatmap-container"),
                ], className="card col-full"),

                # Tabla resumen municipal
                html.Div([
                    html.H3("📊 Resumen por Municipio", className="section-title"),
                    html.Div(id="mongo-muni-table"),
                ], className="card col-full"),

            ], className="dashboard-section"),

            # Comparison Section
            html.Section([
                html.H2("⚖️ Comparación (PostgreSQL vs MongoDB)", className="section-title"),
                html.Div([
                    html.Div(dcc.Graph(figure=make_comparison_bar(total, mongo_total, total_votos, mongo_total_votos), config={"displayModeBar": False},
                                       style={"height": "400px"}), className="card col-full"),
                ], className="row"),
                html.Div([
                    html.H3("📊 Tabla Comparativa Nacional", className="section-title"),
                    html.Div(make_comparison_table(total, mongo_total)),
                ], className="card col-full"),
            ], className="dashboard-section"),

        ], className="main-content"),

        html.Footer([
            html.P("Sistema Electoral RRV · Dashboard Analítico · 2026", className="footer-text"),
        ], className="site-footer"),

    ], className="app-wrapper")

app.layout = serve_layout

# ── Callbacks ─────────────────────────────────────────────────────────────────
@app.callback(Output("heatmap-container", "children"), Input("party-tabs", "value"))
def update_heatmap(party):
    dept_df, _, _, _ = get_processed_data()
    fig = make_heatmap_party(party, dept_df)
    return dcc.Graph(figure=fig, config={"displayModeBar": False}, style={"height": "520px"})


@app.callback(Output("muni-table", "children"), Input("party-tabs", "value"))
def update_table(_):
    _, muni_df, _, _ = get_processed_data()
    rows = []
    for _, r in muni_df.sort_values(["Departamento", "VotosValidos"], ascending=[True, False]).iterrows():
        winner_p = r[PARTY_COLS].idxmax()
        winner_color = PARTY_COLORS[winner_p]
        cells = [
            html.Td(r["Departamento"], className="td-dept"),
            html.Td(r["Municipio"], className="td-muni"),
            *[html.Td(f"{int(r[p]):,}", style={"color": PARTY_COLORS[p], "fontWeight": "600"})
              for p in PARTY_COLS],
            html.Td(f"{int(r['VotosValidos']):,}"),
            html.Td(f"{int(r['VotosBlancos']):,}", style={"color": "#adb5bd"}),
            html.Td(f"{int(r['VotosNulos']):,}", style={"color": "#e63946"}),
            html.Td(f"{r['Participacion']:.1f}%"),
            html.Td(PARTY_NAMES[winner_p],
                    style={"color": winner_color, "fontWeight": "700"}),
        ]
        rows.append(html.Tr(cells))

    header = html.Tr([
        html.Th("Departamento"),
        html.Th("Municipio"),
        *[html.Th(PARTY_NAMES[p]) for p in PARTY_COLS],
        html.Th("Válidos"),
        html.Th("Blancos"),
        html.Th("Nulos"),
        html.Th("Participación"),
        html.Th("Ganador"),
    ])
    return html.Table([html.Thead(header), html.Tbody(rows)], className="muni-table")


@app.callback(Output("mongo-heatmap-container", "children"), Input("mongo-party-tabs", "value"))
def update_mongo_heatmap(party):
    mongo_dept_df, _ = get_mongo_data()
    mongo_dept_df, _, _ = process_df(mongo_dept_df)
    fig = make_heatmap_party(party, mongo_dept_df)
    return dcc.Graph(figure=fig, config={"displayModeBar": False}, style={"height": "520px"})


@app.callback(Output("mongo-muni-table", "children"), Input("mongo-party-tabs", "value"))
def update_mongo_table(_):
    _, mongo_muni_df = get_mongo_data()
    mongo_muni_df, _, _ = process_df(mongo_muni_df)
    rows = []
    for _, r in mongo_muni_df.sort_values(["Departamento", "VotosValidos"], ascending=[True, False]).iterrows():
        winner_p = r[PARTY_COLS].idxmax()
        winner_color = PARTY_COLORS[winner_p]
        cells = [
            html.Td(r["Departamento"], className="td-dept"),
            html.Td(r["Municipio"], className="td-muni"),
            *[html.Td(f"{int(r[p]):,}", style={"color": PARTY_COLORS[p], "fontWeight": "600"})
              for p in PARTY_COLS],
            html.Td(f"{int(r['VotosValidos']):,}"),
            html.Td(f"{int(r['VotosBlancos']):,}", style={"color": "#adb5bd"}),
            html.Td(f"{int(r['VotosNulos']):,}", style={"color": "#e63946"}),
            html.Td(f"{r['Participacion']:.1f}%"),
            html.Td(PARTY_NAMES[winner_p],
                    style={"color": winner_color, "fontWeight": "700"}),
        ]
        rows.append(html.Tr(cells))

    header = html.Tr([
        html.Th("Departamento"),
        html.Th("Municipio"),
        *[html.Th(PARTY_NAMES[p]) for p in PARTY_COLS],
        html.Th("Válidos"),
        html.Th("Blancos"),
        html.Th("Nulos"),
        html.Th("Participación"),
        html.Th("Ganador"),
    ])
    return html.Table([html.Thead(header), html.Tbody(rows)], className="muni-table")


# ── Entry point ───────────────────────────────────────────────────────────────
if __name__ == "__main__":
    port = int(os.environ.get("DASH_PORT", 8050))
    debug = os.environ.get("DASH_DEBUG", "false").lower() == "true"
    app.run(host="0.0.0.0", port=port, debug=debug)
