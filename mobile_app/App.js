import { useState, useRef } from 'react';
import {
  StyleSheet, Text, View, TouchableOpacity, Image,
  SafeAreaView, ActivityIndicator, ScrollView, Alert,
} from 'react-native';
import { CameraView, useCameraPermissions } from 'expo-camera';
import { StatusBar } from 'expo-status-bar';

// ─── CONFIGURACIÓN ────────────────────────────────────────────────────────────
// Cambia esta IP por la de tu computadora en la red local.
// En Android físico/iOS no funciona "localhost" — necesitas la IP real.
// En emulador Android usa: "10.0.2.2"
// Para encontrar tu IP: ejecuta "ipconfig" (Windows) o "ifconfig" (Mac/Linux)
const BACKEND_URL = 'http://192.168.1.100:8080';
// ─────────────────────────────────────────────────────────────────────────────

const ESTADOS = {
  PROCESADA:  { color: '#16a34a', bg: '#dcfce7', label: 'PROCESADA' },
  ERROR:      { color: '#dc2626', bg: '#fee2e2', label: 'ERROR' },
  ANULADA:    { color: '#dc2626', bg: '#fee2e2', label: 'ANULADA' },
  OBSERVADA:  { color: '#d97706', bg: '#fef3c7', label: 'OBSERVADA' },
};

// ─────────────────────────────────────────────────────────────────────────────
// Pantalla: RESULTADOS OCR
// ─────────────────────────────────────────────────────────────────────────────
function ResultsScreen({ acta, onReset }) {
  const estadoInfo = ESTADOS[acta.estado] ?? { color: '#6b7280', bg: '#f3f4f6', label: acta.estado };

  return (
    <SafeAreaView style={styles.container}>
      <StatusBar style="light" />
      <ScrollView contentContainerStyle={styles.resultsScroll} showsVerticalScrollIndicator={false}>

        {/* Badge de estado */}
        <View style={[styles.estadoBadge, { backgroundColor: estadoInfo.bg }]}>
          <Text style={[styles.estadoText, { color: estadoInfo.color }]}>{estadoInfo.label}</Text>
          {acta.motivo_estado ? (
            <Text style={[styles.estadoMotivo, { color: estadoInfo.color }]}>{acta.motivo_estado}</Text>
          ) : null}
        </View>

        {/* Identificación del acta */}
        <Card title="Identificación">
          <Row label="Acta ID"    value={acta.acta_id      || '—'} />
          <Row label="Código mesa" value={acta.codigo_mesa || '—'} />
          <Row label="Mesa"       value={acta.mesa         || '—'} />
        </Card>

        {/* Ubicación */}
        <Card title="Ubicación">
          <Row label="Departamento" value={acta.departamento || '—'} />
          <Row label="Provincia"    value={acta.provincia    || '—'} />
          <Row label="Municipio"    value={acta.municipio    || '—'} />
          <Row label="Recinto"      value={acta.recinto      || '—'} />
        </Card>

        {/* Votantes */}
        {acta.electores_habilitados > 0 && (
          <Card title="Padrón">
            <Row label="Electores habilitados" value={acta.electores_habilitados} />
            <Row label="Papeletas en ánfora"   value={acta.papeletas_anfora} />
            <Row label="Papeletas no usadas"   value={acta.papeletas_no_utilizadas} />
          </Card>
        )}

        {/* Candidatos */}
        {acta.candidatos?.length > 0 && (
          <Card title="Votos por candidato">
            {acta.candidatos.map((c, i) => (
              <Row
                key={c.candidato_id}
                label={candidatoNombre(c.candidato_id, i)}
                value={c.votos}
                highlight
              />
            ))}
          </Card>
        )}

        {/* Totales */}
        <Card title="Totales">
          <Row label="Votos válidos" value={acta.votos_validos}  accent />
          <Row label="Votos blancos" value={acta.votos_blancos} />
          <Row label="Votos nulos"   value={acta.votos_nulos} />
          <View style={styles.divider} />
          <Row label="Total votos"   value={acta.total_votos} accent />
        </Card>

        {/* Validación visual */}
        {acta.validacion_visual && hasVisualFlags(acta.validacion_visual) && (
          <Card title="Alertas visuales" alertCard>
            {acta.validacion_visual.lapiz_detectado      && <Flag label="Uso de lápiz detectado" />}
            {acta.validacion_visual.corrector_detectado  && <Flag label="Corrector (liquid paper)" />}
            {acta.validacion_visual.tachadura_detectada  && <Flag label="Tachadura / sobreescritura" />}
            {acta.validacion_visual.numeros_sobreescritos && <Flag label="Números sobreescritos (OpenCV)" />}
            {acta.validacion_visual.confusion_alfanumerica && <Flag label="Confusión alfanumérica" />}
            {acta.validacion_visual.huellas_zona_numeros  && <Flag label="Huella dactilar en zona de números" />}
            {acta.validacion_visual.rotura_detectada      && <Flag label="Rotura crítica del papel" />}
            {acta.validacion_visual.mancha_detectada && (
              <Flag label={`Mancha de tinta (${acta.validacion_visual.porcentaje_mancha?.toFixed(1)}% del área)`} />
            )}
            {!acta.validacion_visual.firmas_suficientes && (
              <Flag label={`Firmas insuficientes (${acta.validacion_visual.huellas_detectadas} detectadas)`} />
            )}
          </Card>
        )}

        {/* Errores de validación */}
        {acta.errores?.length > 0 && (
          <Card title="Observaciones" alertCard>
            {acta.errores.map((e, i) => (
              <Text key={i} style={styles.errorLine}>• {e}</Text>
            ))}
          </Card>
        )}

        {/* Botón para tomar otra foto */}
        <TouchableOpacity style={styles.btnPrimary} onPress={onReset}>
          <Text style={styles.btnText}>Procesar otra acta</Text>
        </TouchableOpacity>

        <View style={{ height: 32 }} />
      </ScrollView>
    </SafeAreaView>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// Componentes auxiliares de UI
// ─────────────────────────────────────────────────────────────────────────────
function Card({ title, children, alertCard }) {
  return (
    <View style={[styles.card, alertCard && styles.cardAlert]}>
      <Text style={[styles.cardTitle, alertCard && styles.cardTitleAlert]}>{title}</Text>
      {children}
    </View>
  );
}

function Row({ label, value, accent, highlight }) {
  return (
    <View style={styles.row}>
      <Text style={styles.rowLabel}>{label}</Text>
      <Text style={[styles.rowValue, accent && styles.rowValueAccent, highlight && styles.rowValueHighlight]}>
        {value ?? '—'}
      </Text>
    </View>
  );
}

function Flag({ label }) {
  return <Text style={styles.flagLine}>⚠ {label}</Text>;
}

function hasVisualFlags(v) {
  return v.lapiz_detectado || v.corrector_detectado || v.tachadura_detectada ||
    v.numeros_sobreescritos || v.confusion_alfanumerica || v.huellas_zona_numeros ||
    v.rotura_detectada || v.mancha_detectada || !v.firmas_suficientes;
}

const CANDIDATOS_CONOCIDOS = [
  'Daenerys Targaryen',
  'Sansa Stark',
  'Robert Baratheon',
  'Tyrion Lannister',
];
function candidatoNombre(id, idx) {
  return CANDIDATOS_CONOCIDOS[idx] ?? id;
}

// ─────────────────────────────────────────────────────────────────────────────
// Pantalla: CÁMARA
// ─────────────────────────────────────────────────────────────────────────────
export default function App() {
  const [permission, requestPermission] = useCameraPermissions();
  const [photoUri, setPhotoUri] = useState(null);
  const [capturing, setCapturing] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [acta, setActa] = useState(null);
  const cameraRef = useRef(null);

  // ── Sin permiso ──────────────────────────────────────────────────
  if (!permission) return <View style={styles.container} />;
  if (!permission.granted) {
    return (
      <SafeAreaView style={styles.container}>
        <Text style={styles.permissionText}>
          Se necesita acceso a la cámara para fotografiar el acta electoral.
        </Text>
        <TouchableOpacity style={styles.btnPrimary} onPress={requestPermission}>
          <Text style={styles.btnText}>Conceder permiso</Text>
        </TouchableOpacity>
      </SafeAreaView>
    );
  }

  // ── Pantalla de resultados ───────────────────────────────────────
  if (acta) {
    return <ResultsScreen acta={acta} onReset={() => { setActa(null); setPhotoUri(null); }} />;
  }

  // ── Cargando: subiendo y procesando ─────────────────────────────
  if (uploading) {
    return (
      <SafeAreaView style={styles.container}>
        <StatusBar style="light" />
        {photoUri && (
          <Image source={{ uri: photoUri }} style={styles.previewBlurred} resizeMode="cover" blurRadius={4} />
        )}
        <View style={styles.uploadingOverlay}>
          <ActivityIndicator size="large" color="#3b82f6" />
          <Text style={styles.uploadingText}>Procesando OCR…</Text>
          <Text style={styles.uploadingSubtext}>Extrayendo votos y datos del acta</Text>
        </View>
      </SafeAreaView>
    );
  }

  // ── Preview de la foto ───────────────────────────────────────────
  if (photoUri) {
    return (
      <SafeAreaView style={styles.container}>
        <StatusBar style="light" />
        <Text style={styles.previewTitle}>¿La foto es clara?</Text>
        <Image source={{ uri: photoUri }} style={styles.preview} resizeMode="contain" />
        <View style={styles.previewActions}>
          <TouchableOpacity style={styles.btnSecondary} onPress={() => setPhotoUri(null)}>
            <Text style={styles.btnText}>Repetir</Text>
          </TouchableOpacity>
          <TouchableOpacity style={styles.btnPrimary} onPress={() => enviarAlBackend(photoUri)}>
            <Text style={styles.btnText}>Procesar OCR</Text>
          </TouchableOpacity>
        </View>
      </SafeAreaView>
    );
  }

  // ── Cámara ───────────────────────────────────────────────────────
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
        body: form,
        // No pongas 'Content-Type' manualmente — fetch lo genera con el boundary correcto
      });

      const json = await res.json();

      if (res.status === 409) {
        // Duplicado: igual mostramos los datos que ya existen
        setActa(json.data);
        return;
      }
      if (!res.ok || !json.success) {
        const errMsg = json.errors?.join('\n') || json.message || 'Error desconocido';
        Alert.alert('Error de procesamiento', errMsg, [
          { text: 'Reintentar', onPress: () => setUploading(false) },
          { text: 'Descartar', onPress: () => { setPhotoUri(null); setUploading(false); } },
        ]);
        return;
      }

      setActa(json.data);
    } catch (err) {
      Alert.alert(
        'Sin conexión',
        `No se pudo contactar al servidor.\n\nVerifica que el backend esté corriendo y que BACKEND_URL sea correcto.\n\nError: ${err.message}`,
        [
          { text: 'Reintentar', onPress: () => { setUploading(false); enviarAlBackend(uri); } },
          { text: 'Cancelar',   onPress: () => { setPhotoUri(null); setUploading(false); } },
        ],
      );
    } finally {
      setUploading(false);
    }
  }

  return (
    <SafeAreaView style={styles.container}>
      <StatusBar style="light" />
      <Text style={styles.header}>Captura de Acta Electoral</Text>
      <CameraView ref={cameraRef} style={styles.camera} facing="back">
        <View style={styles.guiaOverlay}>
          <View style={styles.guiaMarco} />
        </View>
        <Text style={styles.guiaTxt}>Encuadra el acta dentro del marco</Text>
      </CameraView>
      <View style={styles.controls}>
        <TouchableOpacity
          style={[styles.btnCaptura, capturing && styles.btnDeshabilitado]}
          onPress={tomarFoto}
          disabled={capturing}
        >
          {capturing
            ? <ActivityIndicator color="#fff" size="large" />
            : <View style={styles.btnCapturaInner} />
          }
        </TouchableOpacity>
      </View>
    </SafeAreaView>
  );
}

// ─────────────────────────────────────────────────────────────────────────────
// Estilos
// ─────────────────────────────────────────────────────────────────────────────
const styles = StyleSheet.create({
  container:       { flex: 1, backgroundColor: '#0f172a', alignItems: 'center', justifyContent: 'center' },
  header:          { color: '#e2e8f0', fontSize: 15, fontWeight: '600', letterSpacing: 0.4, paddingVertical: 10 },

  // Cámara
  camera:          { flex: 1, width: '100%' },
  guiaOverlay:     { flex: 1, justifyContent: 'center', alignItems: 'center' },
  guiaMarco:       { width: '88%', height: '78%', borderWidth: 2, borderColor: 'rgba(255,255,255,0.6)', borderRadius: 6, borderStyle: 'dashed' },
  guiaTxt:         { color: 'rgba(255,255,255,0.7)', textAlign: 'center', fontSize: 12, paddingBottom: 8 },
  controls:        { width: '100%', paddingVertical: 22, alignItems: 'center', backgroundColor: '#0f172a' },
  btnCaptura:      { width: 72, height: 72, borderRadius: 36, borderWidth: 3, borderColor: '#fff', justifyContent: 'center', alignItems: 'center' },
  btnCapturaInner: { width: 56, height: 56, borderRadius: 28, backgroundColor: '#fff' },
  btnDeshabilitado:{ opacity: 0.4 },

  // Preview
  previewTitle:    { color: '#e2e8f0', fontSize: 16, fontWeight: '600', marginBottom: 10 },
  preview:         { flex: 1, width: '100%' },
  previewBlurred:  { ...StyleSheet.absoluteFillObject },
  previewActions:  { flexDirection: 'row', gap: 12, paddingVertical: 18, paddingHorizontal: 20 },

  // Upload loading
  uploadingOverlay:{ ...StyleSheet.absoluteFillObject, backgroundColor: 'rgba(15,23,42,0.82)', justifyContent: 'center', alignItems: 'center', gap: 14 },
  uploadingText:   { color: '#f1f5f9', fontSize: 18, fontWeight: '600' },
  uploadingSubtext:{ color: '#94a3b8', fontSize: 13 },

  // Buttons
  btnPrimary:  { flex: 1, backgroundColor: '#2563eb', paddingVertical: 14, borderRadius: 10, alignItems: 'center' },
  btnSecondary:{ flex: 1, backgroundColor: '#334155', paddingVertical: 14, borderRadius: 10, alignItems: 'center' },
  btnText:     { color: '#fff', fontWeight: '600', fontSize: 15 },
  permissionText: { color: '#94a3b8', textAlign: 'center', marginBottom: 24, paddingHorizontal: 32, fontSize: 14, lineHeight: 22 },

  // Results
  resultsScroll: { paddingHorizontal: 16, paddingTop: 12 },

  estadoBadge:  { borderRadius: 10, padding: 14, alignItems: 'center', marginBottom: 12 },
  estadoText:   { fontSize: 20, fontWeight: '800', letterSpacing: 1 },
  estadoMotivo: { fontSize: 12, marginTop: 4, fontWeight: '500' },

  card:       { backgroundColor: '#1e293b', borderRadius: 10, padding: 14, marginBottom: 10 },
  cardAlert:  { backgroundColor: '#1c1917', borderWidth: 1, borderColor: '#854d0e' },
  cardTitle:  { color: '#94a3b8', fontSize: 11, fontWeight: '700', letterSpacing: 0.8, textTransform: 'uppercase', marginBottom: 8 },
  cardTitleAlert: { color: '#ca8a04' },

  row:            { flexDirection: 'row', justifyContent: 'space-between', alignItems: 'center', paddingVertical: 4 },
  rowLabel:       { color: '#64748b', fontSize: 13, flex: 1 },
  rowValue:       { color: '#e2e8f0', fontSize: 14, fontWeight: '500', textAlign: 'right' },
  rowValueAccent: { color: '#60a5fa', fontWeight: '700', fontSize: 16 },
  rowValueHighlight: { color: '#f8fafc', fontWeight: '700' },
  divider:        { height: 1, backgroundColor: '#334155', marginVertical: 6 },

  flagLine:  { color: '#fbbf24', fontSize: 13, paddingVertical: 3 },
  errorLine: { color: '#f87171', fontSize: 12, paddingVertical: 2, lineHeight: 18 },
});
