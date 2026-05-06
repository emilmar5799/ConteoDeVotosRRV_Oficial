import { useState, useRef, useEffect } from 'react';
import {
  StyleSheet, Text, View, TouchableOpacity, Image,
  ActivityIndicator, ScrollView, Alert,
  Platform, StatusBar as RNStatusBar, Animated, Easing
} from 'react-native';
import { CameraView, useCameraPermissions } from 'expo-camera';
import { StatusBar } from 'expo-status-bar';

// ─── CONFIGURACIÓN ────────────────────────────────────────────────────────────
const BACKEND_URL = 'http://192.168.1.73:8080';
// ─────────────────────────────────────────────────────────────────────────────

const ESTADOS = {
  PROCESADA: { color: '#22c55e', bg: '#052e16', border: '#166534', label: 'PROCESADA' },
  ERROR: { color: '#f87171', bg: '#2d0a0a', border: '#7f1d1d', label: 'ERROR' },
  ANULADA: { color: '#f87171', bg: '#2d0a0a', border: '#7f1d1d', label: 'ANULADA' },
  OBSERVADA: { color: '#fbbf24', bg: '#2d1b00', border: '#78350f', label: 'OBSERVADA' },
};

const PARTIDOS = {
  'C1': 'Daenerys Targaryen',
  'C2': 'Sansa Stark',
  'C3': 'Robert Baratheon',
  'C4': 'Tyrion Lannister',
  '1': 'Daenerys Targaryen',
  '2': 'Sansa Stark',
  '3': 'Robert Baratheon',
  '4': 'Tyrion Lannister',
};

function getCandidatoNombre(id) {
  return PARTIDOS[id] || id;
}



// ─────────────────────────────────────────────────────────────────────────────
// Pantalla: RESULTADOS OCR
// ─────────────────────────────────────────────────────────────────────────────
function ResultsScreen({ acta, onReset }) {
  const est = ESTADOS[acta.estado] ?? { color: '#94a3b8', bg: '#1e293b', border: '#334155', label: acta.estado };

  const totalVotos = acta.total_votos || (acta.votos_validos + acta.votos_blancos + acta.votos_nulos);
  const pctValidos = totalVotos > 0 ? ((acta.votos_validos / totalVotos) * 100).toFixed(1) : '0';

  return (
    <View style={s.screen}>
      <StatusBar style="light" />

      {/* Header fijo */}
      <View style={s.resultsHeader}>
        <Text style={s.resultsHeaderTitle}>Resultado OCR</Text>
        <TouchableOpacity style={s.btnNueva} onPress={onReset}>
          <Text style={s.btnNuevaText}>+ Nueva acta</Text>
        </TouchableOpacity>
      </View>

      <ScrollView
        style={s.scroll}
        contentContainerStyle={s.scrollContent}
        showsVerticalScrollIndicator={false}
        bounces={true}
      >
        {/* Badge estado */}
        <View style={[s.estadoBadge, { backgroundColor: est.bg, borderColor: est.border }]}>
          <View style={[s.estadoDot, { backgroundColor: est.color }]} />
          <View style={s.estadoTexts}>
            <Text style={[s.estadoLabel, { color: est.color }]}>{est.label}</Text>
            {acta.motivo_estado ? (
              <Text style={[s.estadoMotivo, { color: est.color }]}>{acta.motivo_estado}</Text>
            ) : null}
          </View>
        </View>

        {/* Identificación */}
        <Section title="Identificación" icon="🪪">
          <DataRow label="Acta ID" value={acta.acta_id} mono />
          <DataRow label="Código mesa" value={acta.codigo_mesa} mono />
          <DataRow label="N° mesa" value={acta.mesa} />
        </Section>

        {/* Ubicación */}
        <Section title="Ubicación" icon="📍">
          <DataRow label="Departamento" value={acta.departamento} />
          <DataRow label="Provincia" value={acta.provincia} />
          <DataRow label="Municipio" value={acta.municipio} />
          <DataRow label="Recinto" value={acta.recinto} />
        </Section>

        {/* Candidatos */}
        {acta.candidatos?.length > 0 && (
          <Section title="Votos por candidato" icon="🗳️">
            {acta.candidatos.map((c, i) => {
              const pct = acta.votos_validos > 0 ? ((c.votos / acta.votos_validos) * 100).toFixed(1) : '0';
              return (
                <View key={c.candidato_id} style={s.candidatoRow}>
                  <View style={s.candidatoLeft}>
                    <View style={[s.candidatoBullet, { backgroundColor: BULLET_COLORS[i % BULLET_COLORS.length] }]} />
                    <Text style={s.candidatoNombre}>
                      {getCandidatoNombre(c.candidato_id) !== c.candidato_id 
                        ? getCandidatoNombre(c.candidato_id) 
                        : (c.partido || c.nombre || c.nombre_partido || `Candidato ${c.candidato_id}`)}
                    </Text>
                  </View>
                  <View style={s.candidatoRight}>
                    <Text style={s.candidatoVotos}>{c.votos}</Text>
                    <Text style={s.candidatoPct}>{pct}%</Text>
                  </View>
                </View>
              );
            })}
            {/* Barra visual de distribución */}
            {acta.candidatos.length > 0 && acta.votos_validos > 0 && (
              <View style={s.barraDistrib}>
                {acta.candidatos.map((c, i) => {
                  const pct = (c.votos / acta.votos_validos) * 100;
                  return (
                    <View
                      key={c.candidato_id}
                      style={[s.barraSegmento, { flex: pct || 0.1, backgroundColor: BULLET_COLORS[i % BULLET_COLORS.length] }]}
                    />
                  );
                })}
              </View>
            )}
          </Section>
        )}

        {/* Totales */}
        <Section title="Totales" icon="📊">
          <DataRow label="Votos válidos" value={acta.votos_validos} accent />
          <DataRow label="Votos blancos" value={acta.votos_blancos} />
          <DataRow label="Votos nulos" value={acta.votos_nulos} />
          <View style={s.divider} />
          <DataRow label="Total sufragios" value={totalVotos} accent />
          <DataRow label="Participación" value={`${pctValidos}% votos válidos`} />
        </Section>

        {/* Padrón */}
        {acta.electores_habilitados > 0 && (
          <Section title="Padrón" icon="👥">
            <DataRow label="Electores habilitados" value={acta.electores_habilitados} />
            <DataRow label="Papeletas en ánfora" value={acta.papeletas_anfora} />
            <DataRow label="Papeletas no utilizadas" value={acta.papeletas_no_utilizadas} />
          </Section>
        )}

        {/* Alertas visuales */}
        {acta.validacion_visual && hasVisualFlags(acta.validacion_visual) && (
          <Section title="Alertas visuales" icon="⚠️" alert>
            {acta.validacion_visual.lapiz_detectado && <AlertRow label="Escritura a lápiz detectada" />}
            {acta.validacion_visual.corrector_detectado && <AlertRow label="Corrector (liquid paper)" />}
            {acta.validacion_visual.tachadura_detectada && <AlertRow label="Tachadura / sobreescritura" />}
            {acta.validacion_visual.numeros_sobreescritos && <AlertRow label="Números sobreescritos (OpenCV)" />}
            {acta.validacion_visual.confusion_alfanumerica && <AlertRow label="Confusión alfanumérica" />}
            {acta.validacion_visual.huellas_zona_numeros && <AlertRow label="Huella dactilar en zona de números" />}
            {acta.validacion_visual.rotura_detectada && <AlertRow label="Rotura crítica del papel" />}
            {acta.validacion_visual.mancha_detectada && (
              <AlertRow label={`Mancha de tinta (${acta.validacion_visual.porcentaje_mancha?.toFixed(1)}% del área)`} />
            )}
            {!acta.validacion_visual.firmas_suficientes && (
              <AlertRow label={`Firmas insuficientes (${acta.validacion_visual.huellas_detectadas} detectadas)`} />
            )}
          </Section>
        )}

        {/* Observaciones */}
        {acta.errores?.length > 0 && (
          <Section title="Observaciones" icon="📋" alert>
            {acta.errores.map((e, i) => (
              <Text key={i} style={s.errorLine}>• {e}</Text>
            ))}
          </Section>
        )}

        <View style={{ height: 40 }} />
      </ScrollView>
    </View>
  );
}

const BULLET_COLORS = ['#3b82f6', '#f59e0b', '#10b981', '#ef4444', '#8b5cf6', '#06b6d4'];

function Section({ title, icon, children, alert }) {
  return (
    <View style={[s.section, alert && s.sectionAlert]}>
      <View style={s.sectionHeader}>
        <Text style={s.sectionIcon}>{icon}</Text>
        <Text style={[s.sectionTitle, alert && s.sectionTitleAlert]}>{title}</Text>
      </View>
      <View style={s.sectionBody}>{children}</View>
    </View>
  );
}

function DataRow({ label, value, accent, mono }) {
  if (value === null || value === undefined || value === '' || value === 0) {
    return (
      <View style={s.dataRow}>
        <Text style={s.dataLabel}>{label}</Text>
        <Text style={s.dataEmpty}>—</Text>
      </View>
    );
  }
  return (
    <View style={s.dataRow}>
      <Text style={s.dataLabel}>{label}</Text>
      <Text style={[s.dataValue, accent && s.dataValueAccent, mono && s.dataValueMono]}>
        {typeof value === 'number' ? value.toLocaleString() : value}
      </Text>
    </View>
  );
}

function AlertRow({ label }) {
  return (
    <View style={s.alertRow}>
      <Text style={s.alertDot}>▲</Text>
      <Text style={s.alertText}>{label}</Text>
    </View>
  );
}

function hasVisualFlags(v) {
  return v.lapiz_detectado || v.corrector_detectado || v.tachadura_detectada ||
    v.numeros_sobreescritos || v.confusion_alfanumerica || v.huellas_zona_numeros ||
    v.rotura_detectada || v.mancha_detectada || !v.firmas_suficientes;
}

// ─────────────────────────────────────────────────────────────────────────────
// Pantalla principal: CÁMARA + PREVIEW + CARGA
// ─────────────────────────────────────────────────────────────────────────────
export default function App() {
  const [permission, requestPermission] = useCameraPermissions();
  const [photoUri, setPhotoUri] = useState(null);
  const [capturing, setCapturing] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [acta, setActa] = useState(null);
  const cameraRef = useRef(null);

  // Progress bar animation
  const progressAnim = useRef(new Animated.Value(0)).current;

  useEffect(() => {
    if (uploading) {
      progressAnim.setValue(0);
      Animated.loop(
        Animated.timing(progressAnim, {
          toValue: 1,
          duration: 1200,
          easing: Easing.inOut(Easing.ease),
          useNativeDriver: false,
        })
      ).start();
    } else {
      progressAnim.stopAnimation();
    }
  }, [uploading]);

  const barWidth = progressAnim.interpolate({
    inputRange: [0, 0.5, 1],
    outputRange: ['0%', '80%', '0%']
  });
  const barLeft = progressAnim.interpolate({
    inputRange: [0, 0.5, 1],
    outputRange: ['0%', '10%', '100%']
  });

  if (!permission) return <View style={s.screen} />;

  if (!permission.granted) {
    return (
      <View style={s.screen}>
        <StatusBar style="light" />
        <View style={s.permissionBox}>
          <Text style={s.permissionIcon}>📷</Text>
          <Text style={s.permissionTitle}>Permiso de cámara</Text>
          <Text style={s.permissionDesc}>
            Para fotografiar el acta electoral y procesarla con OCR.
          </Text>
          <TouchableOpacity style={s.btnPrimary} onPress={requestPermission}>
            <Text style={s.btnPrimaryText}>Conceder permiso</Text>
          </TouchableOpacity>
        </View>
      </View>
    );
  }

  if (acta) {
    return <ResultsScreen acta={acta} onReset={() => {
      setActa(null);
      setPhotoUri(null);
    }} />;
  }

  if (uploading) {
    return (
      <View style={s.screen}>
        <StatusBar style="light" />
        {photoUri && (
          <Image source={{ uri: photoUri }} style={StyleSheet.absoluteFill} resizeMode="cover" blurRadius={6} />
        )}
        <View style={s.loadingOverlay}>
          <View style={s.loadingCard}>
            <Text style={s.loadingTitle}>Procesando OCR…</Text>
            <Text style={s.loadingDesc}>Extrayendo votos y datos del acta</Text>

            <View style={s.progressBarWrap}>
              <Animated.View style={[s.progressBar, { width: barWidth, left: barLeft }]} />
            </View>
          </View>
        </View>
      </View>
    );
  }

  if (photoUri) {
    return (
      <View style={s.screen}>
        <StatusBar style="light" />
        <Text style={s.previewTitle}>¿La foto es clara?</Text>
        <Text style={s.previewSubtitle}>Verifica que los números sean legibles</Text>
        <Image source={{ uri: photoUri }} style={s.previewImage} resizeMode="contain" />
        <View style={s.previewActions}>
          <TouchableOpacity style={s.btnSecondary} onPress={() => setPhotoUri(null)}>
            <Text style={s.btnSecondaryText}>↩ Repetir</Text>
          </TouchableOpacity>
          <TouchableOpacity style={s.btnPrimary} onPress={() => enviarAlBackend(photoUri)}>
            <Text style={s.btnPrimaryText}>Procesar OCR →</Text>
          </TouchableOpacity>
        </View>
      </View>
    );
  }

  async function tomarFoto() {
    if (!cameraRef.current || capturing) return;
    setCapturing(true);
    try {
      const foto = await cameraRef.current.takePictureAsync({ quality: 0.95, skipProcessing: false });
      setPhotoUri(foto.uri);
    } catch {
      Alert.alert('Error', 'No se pudo capturar la foto.');
    } finally {
      setCapturing(false);
    }
  }

  async function enviarAlBackend(uri) {
    setUploading(true);
    try {
      const form = new FormData();
      const filename = 'acta_' + Date.now() + '.jpg';
      form.append('archivo', { uri, name: filename, type: 'image/jpeg' });

      const res = await fetch(`${BACKEND_URL}/api/rrv/actas/upload`, {
        method: 'POST',
        body: form
      });

      const json = await res.json();

      if (res.status === 409) { setActa(json.data); return; }
      if (!res.ok || !json.success) {
        const errMsg = json.errors?.join('\n') || json.message || 'Error desconocido';
        Alert.alert('Error de procesamiento', errMsg, [
          { text: 'Reintentar', onPress: () => setUploading(false) },
          { text: 'Descartar', onPress: () => { setPhotoUri(null); setUploading(false); } },
        ]);
        return;
      }
      
      const actaObj = json.data;
      
      const noHayVotos = (actaObj.votos_validos === 0 && actaObj.votos_nulos === 0 && actaObj.votos_blancos === 0);

      if (noHayVotos) {
        Alert.alert(
          'Imagen no detectada',
          'No se pudieron extraer los datos básicos del acta.\n\nPor favor intenta nuevamente:\n• No muevas la cámara al tomar la foto\n• Verifica que haya buena iluminación\n• Asegúrate de que los números sean claros',
          [
            { text: 'Tomar otra foto', onPress: () => { setPhotoUri(null); setUploading(false); } }
          ]
        );
        return;
      }

      setActa(actaObj);
    } catch (err) {
      const msg = `No se pudo contactar al servidor.\n\nVerifica que:\n• El backend esté corriendo\n• BACKEND_URL sea tu IP local\n\n${err.message}`;

      Alert.alert('Error de conexión', msg, [
        { text: 'Reintentar', onPress: () => { setUploading(false); enviarAlBackend(uri); } },
        { text: 'Cancelar', onPress: () => { setPhotoUri(null); setUploading(false); } },
      ]);
    } finally {
      setUploading(false);
    }
  }

  // ── Pantalla de cámara ────────────────────────────────────────────
  return (
    <View style={s.screen}>
      <StatusBar style="light" />

      {/* Header */}
      <View style={s.camHeader}>
        <Text style={s.camHeaderTitle}>Captura de Acta Electoral</Text>
        <Text style={s.camHeaderSub}>Centra el documento dentro del marco</Text>
      </View>

      {/* Cámara con overlay de escaneo */}
      <View style={s.cameraWrap}>
        <CameraView ref={cameraRef} style={StyleSheet.absoluteFill} facing="back" />

        {/* Oscurecimiento lateral — deja el área del acta clara */}
        <View style={s.scanOverlay} pointerEvents="none">
          {/* Fila superior oscura */}
          <View style={s.scanDark} />
          {/* Fila central: lateral izq + ventana clara + lateral der */}
          <View style={s.scanRow}>
            <View style={s.scanSide} />
            <View style={s.scanWindow}>
              {/* Esquinas decorativas */}
              <View style={[s.corner, s.cornerTL]} />
              <View style={[s.corner, s.cornerTR]} />
              <View style={[s.corner, s.cornerBL]} />
              <View style={[s.corner, s.cornerBR]} />
            </View>
            <View style={s.scanSide} />
          </View>
          {/* Fila inferior oscura */}
          <View style={s.scanDark} />
        </View>

        {/* Instrucción flotante */}
        <View style={s.scanHint} pointerEvents="none">
          <Text style={s.scanHintText}>Mantén el acta plana y bien iluminada</Text>
        </View>
      </View>

      {/* Botón de captura */}
      <View style={s.camFooter}>
        <TouchableOpacity
          style={[s.captureBtn, capturing && s.captureBtnDisabled]}
          onPress={tomarFoto}
          disabled={capturing}
          activeOpacity={0.8}
        >
          {capturing
            ? <ActivityIndicator color="#fff" size="large" />
            : <View style={s.captureBtnInner} />
          }
        </TouchableOpacity>
      </View>
    </View>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// Estilos
// ─────────────────────────────────────────────────────────────────────────────
const BG = '#0a0f1e';
const CARD = '#111827';
const BORDER = '#1e2a3d';
const TEXT = '#f1f5f9';
const MUTED = '#64748b';
const ACCENT = '#3b82f6';

const STATUSBAR_HEIGHT = Platform.OS === 'android' ? RNStatusBar.currentHeight : 44;

const s = StyleSheet.create({
  screen: { flex: 1, backgroundColor: BG, paddingTop: STATUSBAR_HEIGHT },

  // ── Cámara ────────────────────────────────────────────────────────
  camHeader: {
    paddingTop: Platform.OS === 'android' ? 12 : 4,
    paddingBottom: 10,
    paddingHorizontal: 20,
    alignItems: 'center',
  },
  camHeaderTitle: { color: TEXT, fontSize: 16, fontWeight: '700', letterSpacing: 0.3 },
  camHeaderSub: { color: MUTED, fontSize: 12, marginTop: 2 },

  cameraWrap: { flex: 1, position: 'relative', overflow: 'hidden' },

  // Overlay de escaneo
  scanOverlay: { ...StyleSheet.absoluteFillObject, flexDirection: 'column' },
  scanDark: { flex: 1, backgroundColor: 'rgba(0,0,0,0.55)' },
  scanRow: { flexDirection: 'row', height: '62%' },
  scanSide: { flex: 1, backgroundColor: 'rgba(0,0,0,0.55)' },
  scanWindow: {
    flex: 7,
    borderWidth: 0,
    position: 'relative',
  },

  // Esquinas del visor
  corner: { position: 'absolute', width: 22, height: 22, borderColor: '#fff' },
  cornerTL: { top: 0, left: 0, borderTopWidth: 3, borderLeftWidth: 3, borderTopLeftRadius: 3 },
  cornerTR: { top: 0, right: 0, borderTopWidth: 3, borderRightWidth: 3, borderTopRightRadius: 3 },
  cornerBL: { bottom: 0, left: 0, borderBottomWidth: 3, borderLeftWidth: 3, borderBottomLeftRadius: 3 },
  cornerBR: { bottom: 0, right: 0, borderBottomWidth: 3, borderRightWidth: 3, borderBottomRightRadius: 3 },

  scanHint: {
    position: 'absolute',
    bottom: 12,
    left: 0, right: 0,
    alignItems: 'center',
  },
  scanHintText: {
    color: 'rgba(255,255,255,0.8)',
    fontSize: 12,
    backgroundColor: 'rgba(0,0,0,0.4)',
    paddingHorizontal: 12,
    paddingVertical: 4,
    borderRadius: 20,
    overflow: 'hidden',
  },
  scanHintError: {
    color: '#f87171',
    backgroundColor: 'rgba(127,29,29,0.85)',
    fontWeight: '600',
  },

  camFooter: {
    paddingVertical: 24,
    alignItems: 'center',
    backgroundColor: BG,
  },
  captureBtn: {
    width: 76,
    height: 76,
    borderRadius: 38,
    borderWidth: 3,
    borderColor: '#fff',
    justifyContent: 'center',
    alignItems: 'center',
    backgroundColor: 'rgba(255,255,255,0.08)',
  },
  captureBtnDisabled: { opacity: 0.4 },
  captureBtnInner: {
    width: 58,
    height: 58,
    borderRadius: 29,
    backgroundColor: '#fff',
  },

  // ── Preview ───────────────────────────────────────────────────────
  previewTitle: { color: TEXT, fontSize: 17, fontWeight: '700', textAlign: 'center', marginTop: 16, marginBottom: 4 },
  previewSubtitle: { color: MUTED, fontSize: 13, textAlign: 'center', marginBottom: 12 },
  previewImage: { flex: 1, width: '100%' },
  previewActions: {
    flexDirection: 'row',
    gap: 12,
    padding: 16,
    backgroundColor: BG,
  },

  // ── Carga ─────────────────────────────────────────────────────────
  loadingOverlay: {
    ...StyleSheet.absoluteFillObject,
    justifyContent: 'center',
    alignItems: 'center',
    backgroundColor: 'rgba(10,15,30,0.75)',
  },
  loadingCard: {
    backgroundColor: CARD,
    borderRadius: 16,
    padding: 32,
    alignItems: 'center',
    gap: 14,
    borderWidth: 1,
    borderColor: BORDER,
    minWidth: 220,
  },
  loadingTitle: { color: TEXT, fontSize: 17, fontWeight: '600' },
  loadingDesc: { color: MUTED, fontSize: 13, marginBottom: 8 },
  progressBarWrap: {
    height: 6,
    width: '100%',
    backgroundColor: 'rgba(255,255,255,0.1)',
    borderRadius: 3,
    overflow: 'hidden',
  },
  progressBar: {
    position: 'absolute',
    height: '100%',
    backgroundColor: ACCENT,
    borderRadius: 3,
  },

  // ── Permiso ───────────────────────────────────────────────────────
  permissionBox: {
    flex: 1, justifyContent: 'center', alignItems: 'center',
    paddingHorizontal: 32, gap: 14,
  },
  permissionIcon: { fontSize: 48 },
  permissionTitle: { color: TEXT, fontSize: 20, fontWeight: '700' },
  permissionDesc: { color: MUTED, fontSize: 14, textAlign: 'center', lineHeight: 21 },

  // ── Botones ───────────────────────────────────────────────────────
  btnPrimary: {
    flex: 1,
    backgroundColor: ACCENT,
    paddingVertical: 14,
    borderRadius: 12,
    alignItems: 'center',
  },
  btnPrimaryText: { color: '#fff', fontWeight: '700', fontSize: 15 },
  btnSecondary: {
    flex: 1,
    backgroundColor: '#1e293b',
    paddingVertical: 14,
    borderRadius: 12,
    alignItems: 'center',
    borderWidth: 1,
    borderColor: BORDER,
  },
  btnSecondaryText: { color: '#cbd5e1', fontWeight: '600', fontSize: 15 },

  // ── Resultados: layout ────────────────────────────────────────────
  resultsHeader: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    paddingHorizontal: 16,
    paddingTop: Platform.OS === 'android' ? 12 : 4,
    paddingBottom: 10,
    borderBottomWidth: 1,
    borderBottomColor: BORDER,
  },
  resultsHeaderTitle: { color: TEXT, fontSize: 16, fontWeight: '700' },
  btnNueva: {
    backgroundColor: '#1e2a3d',
    paddingHorizontal: 12,
    paddingVertical: 6,
    borderRadius: 8,
    borderWidth: 1,
    borderColor: ACCENT,
  },
  btnNuevaText: { color: ACCENT, fontSize: 13, fontWeight: '600' },

  scroll: { flex: 1 },
  scrollContent: { padding: 14, paddingBottom: 8 },

  // ── Estado badge ──────────────────────────────────────────────────
  estadoBadge: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 12,
    borderRadius: 12,
    padding: 14,
    marginBottom: 12,
    borderWidth: 1,
  },
  estadoDot: { width: 12, height: 12, borderRadius: 6 },
  estadoTexts: { flex: 1 },
  estadoLabel: { fontSize: 16, fontWeight: '800', letterSpacing: 0.8 },
  estadoMotivo: { fontSize: 11, marginTop: 2, fontWeight: '500' },

  // ── Secciones ─────────────────────────────────────────────────────
  section: {
    backgroundColor: CARD,
    borderRadius: 12,
    marginBottom: 10,
    borderWidth: 1,
    borderColor: BORDER,
    overflow: 'hidden',
  },
  sectionAlert: { borderColor: '#78350f' },
  sectionHeader: {
    flexDirection: 'row',
    alignItems: 'center',
    gap: 6,
    paddingHorizontal: 14,
    paddingVertical: 10,
    borderBottomWidth: 1,
    borderBottomColor: BORDER,
    backgroundColor: 'rgba(255,255,255,0.03)',
  },
  sectionIcon: { fontSize: 14 },
  sectionTitle: { color: '#94a3b8', fontSize: 11, fontWeight: '700', letterSpacing: 0.8, textTransform: 'uppercase' },
  sectionTitleAlert: { color: '#f59e0b' },
  sectionBody: { paddingHorizontal: 14, paddingVertical: 8 },

  // ── Filas de datos ────────────────────────────────────────────────
  dataRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    paddingVertical: 6,
    borderBottomWidth: 1,
    borderBottomColor: 'rgba(255,255,255,0.04)',
  },
  dataLabel: { color: MUTED, fontSize: 13, flex: 1, marginRight: 8 },
  dataValue: { color: TEXT, fontSize: 14, fontWeight: '500', textAlign: 'right', maxWidth: '60%' },
  dataValueAccent: { color: '#60a5fa', fontWeight: '700', fontSize: 16 },
  dataValueMono: { fontFamily: Platform.OS === 'ios' ? 'Menlo' : 'monospace', fontSize: 12 },
  dataEmpty: { color: '#374151', fontSize: 14, textAlign: 'right' },
  divider: { height: 1, backgroundColor: BORDER, marginVertical: 4 },

  // ── Candidatos ────────────────────────────────────────────────────
  candidatoRow: {
    flexDirection: 'row',
    justifyContent: 'space-between',
    alignItems: 'center',
    paddingVertical: 8,
    borderBottomWidth: 1,
    borderBottomColor: 'rgba(255,255,255,0.04)',
  },
  candidatoLeft: { flexDirection: 'row', alignItems: 'center', gap: 8, flex: 1 },
  candidatoBullet: { width: 10, height: 10, borderRadius: 5 },
  candidatoNombre: { color: TEXT, fontSize: 14, fontWeight: '500' },
  candidatoRight: { alignItems: 'flex-end' },
  candidatoVotos: { color: '#f8fafc', fontSize: 18, fontWeight: '700' },
  candidatoPct: { color: MUTED, fontSize: 11, marginTop: 1 },

  barraDistrib: {
    flexDirection: 'row',
    height: 6,
    borderRadius: 3,
    overflow: 'hidden',
    marginTop: 12,
    marginBottom: 4,
  },
  barraSegmento: { height: 6 },

  // ── Alertas ───────────────────────────────────────────────────────
  alertRow: { flexDirection: 'row', alignItems: 'flex-start', gap: 8, paddingVertical: 5 },
  alertDot: { color: '#f59e0b', fontSize: 10, marginTop: 2 },
  alertText: { color: '#fde68a', fontSize: 13, flex: 1, lineHeight: 19 },
  errorLine: { color: '#fca5a5', fontSize: 12, paddingVertical: 3, lineHeight: 18 },
});
