import React, { useState } from 'react';
import axios from 'axios';
import { User, FileText, CheckCircle, AlertTriangle } from 'lucide-react';

const API_URL = '/api';

function App() {
  const [formData, setFormData] = useState({
    funcionario: '',
    codigoActa: '',
    codigoRecinto: '',
    nroMesa: '',
    p1: 0, p2: 0, p3: 0, p4: 0,
    votosValidos: 0,
    blancos: 0, nulos: 0,
    anfora: 0, noUtilizadas: 0,
    aperturaHora: 8, aperturaMinutos: 0,
    cierreHora: 16, cierreMinutos: 0,
    observaciones: ''
  });

  const [errors, setErrors] = useState({});
  const [submitError, setSubmitError] = useState('');
  const [submitSuccess, setSubmitSuccess] = useState('');

  const handleNumberChange = (e) => {
    const { name, value } = e.target;
    setFormData(prev => ({ ...prev, [name]: parseInt(value) || 0 }));
  };

  const handleTextChange = (e) => {
    const { name, value } = e.target;
    setFormData(prev => ({ ...prev, [name]: value }));
  };

  const handleSubmit = async (e) => {
    e.preventDefault();
    setErrors({});
    setSubmitError('');
    setSubmitSuccess('');

    let newErrors = {};

    // Validaciones Matemáticas
    const sumPartidos = formData.p1 + formData.p2 + formData.p3 + formData.p4;
    if (formData.votosValidos !== sumPartidos) {
      newErrors.votosValidos = `La suma de partidos (${sumPartidos}) no coincide con los Votos Válidos.`;
    }

    const sumAnfora = formData.votosValidos + formData.blancos + formData.nulos;
    if (formData.anfora !== sumAnfora) {
      newErrors.anfora = `La suma (Válidos + Blancos + Nulos = ${sumAnfora}) no coincide con el ánfora.`;
    }

    if (Object.keys(newErrors).length > 0) {
      setErrors(newErrors);
      return;
    }

    // 1. Validate Funcionario
    let userId;
    try {
      const userRes = await axios.post(`${API_URL}/login`, { nombre: formData.funcionario });
      userId = userRes.data.id_usuario;
    } catch (err) {
      newErrors.funcionario = err.response?.data?.error || 'Usuario no encontrado';
    }

    // 2. Validate Acta (codigo_acta tiene prioridad; fallback a recinto+mesa)
    let acta;
    const params = formData.codigoActa
      ? { codigo_acta: formData.codigoActa }
      : { codigo_recinto: formData.codigoRecinto, nro_mesa: formData.nroMesa };

    if (formData.codigoActa || (formData.codigoRecinto && formData.nroMesa)) {
      try {
        const actaRes = await axios.get(`${API_URL}/actas`, { params });
        acta = actaRes.data;

        // 3. Limit validation (Total = Anfora + NoUtilizadas)
        const totalPapeletas = formData.anfora + formData.noUtilizadas;
        if (totalPapeletas !== acta.votantes_habilitados) {
          newErrors.noUtilizadas = `Las papeletas totales (${totalPapeletas}) no coinciden con los habilitados (${acta.votantes_habilitados}).`;
        }
      } catch (err) {
        newErrors.mesa = err.response?.data?.error || 'Acta no encontrada';
      }
    } else {
      newErrors.mesa = 'Ingrese el Código de Acta, o bien el Recinto y Número de Mesa.';
    }

    if (Object.keys(newErrors).length > 0) {
      setErrors(newErrors);
      return;
    }

    // If everything is correct, Submit!
    const payload = {
      codigo_acta: acta.codigo_acta,
      id_usuario: userId,
      p1: formData.p1,
      p2: formData.p2,
      p3: formData.p3,
      p4: formData.p4,
      votos_validos: formData.votosValidos,
      votos_blancos: formData.blancos,
      votos_nulos: formData.nulos,
      papeletas_anfora: formData.anfora,
      papeletas_no_utilizadas: formData.noUtilizadas,
      apertura_hora: formData.aperturaHora,
      apertura_minutos: formData.aperturaMinutos,
      cierre_hora: formData.cierreHora,
      cierre_minutos: formData.cierreMinutos,
      observaciones: formData.observaciones
    };

    try {
      await axios.post(`${API_URL}/transcripciones`, payload);
      setSubmitSuccess('Acta transcrita exitosamente.');
      setFormData({
        funcionario: formData.funcionario, // Keep user
        codigoActa: '',
        codigoRecinto: '', nroMesa: '',
        p1: 0, p2: 0, p3: 0, p4: 0,
        votosValidos: 0,
        blancos: 0, nulos: 0,
        anfora: 0, noUtilizadas: 0,
        aperturaHora: 8, aperturaMinutos: 0,
        cierreHora: 16, cierreMinutos: 0,
        observaciones: ''
      });
      window.scrollTo(0, 0);
    } catch (err) {
      setSubmitError(err.response?.data?.error || 'Error al guardar la transcripción');
    }
  };

  return (
    <div className="container" style={{ paddingBottom: '3rem' }}>
      <div className="glass-panel" style={{ maxWidth: '800px', margin: '0 auto' }}>
        <div style={{ textAlign: 'center', marginBottom: '2rem' }}>
          <FileText size={48} color="var(--accent)" style={{ marginBottom: '1rem' }} />
          <h1>Formulario de Transcripción</h1>
          <p className="subtitle">Complete todos los datos del acta escrutada.</p>
        </div>

        {submitSuccess && (
          <div className="alert alert-success" style={{ marginBottom: '2rem' }}>
            <CheckCircle size={20} />
            <span>{submitSuccess}</span>
          </div>
        )}

        {submitError && (
          <div className="alert alert-error" style={{ marginBottom: '2rem' }}>
            <AlertTriangle size={20} />
            <span>{submitError}</span>
          </div>
        )}

        <form onSubmit={handleSubmit}>
          
          {/* SECCION USUARIO */}
          <div className="card-info" style={{ marginBottom: '2rem' }}>
            <h2 style={{ fontSize: '1.2rem', marginBottom: '1rem', display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
              <User size={20} /> Identificación del Funcionario
            </h2>
            <div className="form-group">
              <label>Nombre del Funcionario</label>
              <input 
                type="text" 
                name="funcionario"
                value={formData.funcionario} 
                onChange={handleTextChange} 
                placeholder="Ej. Funcionario 1"
                required 
              />
              {errors.funcionario && <span className="error-text" style={{ fontSize: '0.85rem', marginTop: '0.2rem', display: 'block' }}>{errors.funcionario}</span>}
            </div>
          </div>

          {/* SECCION ACTA */}
          <div className="card-info" style={{ marginBottom: '2rem' }}>
            <h2 style={{ fontSize: '1.2rem', marginBottom: '1rem', display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
              <FileText size={20} /> Datos del Acta
            </h2>

            <div className="form-group">
              <label>Código de Acta</label>
              <input 
                type="number" 
                name="codigoActa"
                value={formData.codigoActa} 
                onChange={handleTextChange} 
                placeholder="Ej. 1010200001001"
              />
            </div>

            <div style={{ textAlign: 'center', margin: '1rem 0', color: 'var(--text-muted, #666)', fontSize: '0.9rem' }}>
              — o localizar por —
            </div>

            <div className="grid-2">
              <div className="form-group">
                <label>Código de Recinto</label>
                <input 
                  type="number" 
                  name="codigoRecinto"
                  value={formData.codigoRecinto} 
                  onChange={handleTextChange} 
                  placeholder="Ej. 1010200001"
                />
              </div>
              <div className="form-group">
                <label>Número de Mesa</label>
                <input 
                  type="number" 
                  name="nroMesa"
                  value={formData.nroMesa} 
                  onChange={handleTextChange} 
                  placeholder="Ej. 1"
                />
              </div>
            </div>
            {errors.mesa && <span className="error-text" style={{ fontSize: '0.85rem', marginTop: '0.2rem', display: 'block' }}>{errors.mesa}</span>}
          </div>

          {/* SECCION VOTOS */}
          <div className="grid-2">
            <div>
              <h2 style={{ fontSize: '1.2rem', marginBottom: '1rem' }}>Votos por Partido</h2>
              <div className="form-group">
                <label>Partido 1 (P1)</label>
                <input type="number" name="p1" value={formData.p1} onChange={handleNumberChange} min="0" required />
              </div>
              <div className="form-group">
                <label>Partido 2 (P2)</label>
                <input type="number" name="p2" value={formData.p2} onChange={handleNumberChange} min="0" required />
              </div>
              <div className="form-group">
                <label>Partido 3 (P3)</label>
                <input type="number" name="p3" value={formData.p3} onChange={handleNumberChange} min="0" required />
              </div>
              <div className="form-group">
                <label>Partido 4 (P4)</label>
                <input type="number" name="p4" value={formData.p4} onChange={handleNumberChange} min="0" required />
              </div>
              <div className="form-group" style={{ marginTop: '1rem', borderTop: '1px solid var(--border)', paddingTop: '1rem' }}>
                <label style={{ color: 'var(--accent)' }}>Votos Válidos</label>
                <input 
                  type="number" 
                  name="votosValidos" 
                  value={formData.votosValidos} 
                  onChange={handleNumberChange} 
                  min="0" 
                  required 
                  style={errors.votosValidos ? { borderColor: 'var(--error)' } : {}}
                />
                {errors.votosValidos && <span className="error-text" style={{ fontSize: '0.85rem', marginTop: '0.2rem', display: 'block' }}>{errors.votosValidos}</span>}
              </div>
            </div>

            <div>
              <h2 style={{ fontSize: '1.2rem', marginBottom: '1rem' }}>Otros Votos e Información</h2>
              <div className="form-group">
                <label>Votos Blancos</label>
                <input type="number" name="blancos" value={formData.blancos} onChange={handleNumberChange} min="0" required />
              </div>
              <div className="form-group">
                <label>Votos Nulos</label>
                <input type="number" name="nulos" value={formData.nulos} onChange={handleNumberChange} min="0" required />
              </div>
              <div className="form-group" style={{ marginTop: '2rem', borderTop: '1px solid var(--border)', paddingTop: '1rem' }}>
                <label style={{ color: 'var(--accent)' }}>Papeletas en Ánfora</label>
                <input 
                  type="number" 
                  name="anfora" 
                  value={formData.anfora} 
                  onChange={handleNumberChange} 
                  min="0" 
                  required 
                  style={errors.anfora ? { borderColor: 'var(--error)' } : {}}
                />
                {errors.anfora && <span className="error-text" style={{ fontSize: '0.85rem', marginTop: '0.2rem', display: 'block' }}>{errors.anfora}</span>}
              </div>
              <div className="form-group">
                <label>Papeletas No Utilizadas</label>
                <input 
                  type="number" 
                  name="noUtilizadas" 
                  value={formData.noUtilizadas} 
                  onChange={handleNumberChange} 
                  min="0" 
                  required 
                  style={errors.noUtilizadas ? { borderColor: 'var(--error)' } : {}}
                />
                {errors.noUtilizadas && <span className="error-text" style={{ fontSize: '0.85rem', marginTop: '0.2rem', display: 'block' }}>{errors.noUtilizadas}</span>}
              </div>
            </div>
          </div>

          {/* SECCION TIEMPOS */}
          <div className="card-info" style={{ marginTop: '2rem' }}>
            <h2 style={{ fontSize: '1.2rem', marginBottom: '1rem', display: 'flex', alignItems: 'center', gap: '0.5rem' }}>
               Horarios de la Jornada
            </h2>
            <div className="grid-2">
              <div>
                <h3 style={{ fontSize: '1rem', marginBottom: '0.5rem' }}>Apertura</h3>
                <div style={{ display: 'flex', gap: '1rem' }}>
                  <div className="form-group" style={{ flex: 1 }}>
                    <label>Hora</label>
                    <input type="number" name="aperturaHora" value={formData.aperturaHora} onChange={handleNumberChange} min="0" max="23" required />
                  </div>
                  <div className="form-group" style={{ flex: 1 }}>
                    <label>Minutos</label>
                    <input type="number" name="aperturaMinutos" value={formData.aperturaMinutos} onChange={handleNumberChange} min="0" max="59" required />
                  </div>
                </div>
              </div>
              <div>
                <h3 style={{ fontSize: '1rem', marginBottom: '0.5rem' }}>Cierre</h3>
                <div style={{ display: 'flex', gap: '1rem' }}>
                  <div className="form-group" style={{ flex: 1 }}>
                    <label>Hora</label>
                    <input type="number" name="cierreHora" value={formData.cierreHora} onChange={handleNumberChange} min="0" max="23" required />
                  </div>
                  <div className="form-group" style={{ flex: 1 }}>
                    <label>Minutos</label>
                    <input type="number" name="cierreMinutos" value={formData.cierreMinutos} onChange={handleNumberChange} min="0" max="59" required />
                  </div>
                </div>
              </div>
            </div>
          </div>

          <div className="form-group" style={{ marginTop: '2rem' }}>
            <label>Observaciones (Opcional)</label>
            <textarea 
              name="observaciones" 
              value={formData.observaciones} 
              onChange={handleTextChange} 
              rows="3"
              placeholder="Ingrese observaciones del acta si las hubiera..."
            ></textarea>
          </div>

          <button type="submit" className="btn" style={{ marginTop: '2rem', width: '100%' }}>
            Enviar Transcripción Oficial
          </button>
        </form>
      </div>
    </div>
  );
}

export default App;
