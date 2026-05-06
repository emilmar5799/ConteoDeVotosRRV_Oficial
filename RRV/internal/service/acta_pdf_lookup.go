package service

import (
	"fmt"
	"os"
	"path/filepath"

	"rrv-backend/internal/models"
)

func (p *OCRPipeline) completarDesdePDFPorCodigo(acta *models.ActaRRV) (bool, error) {
	if acta == nil || acta.CodigoMesa == "" {
		return false, nil
	}

	pdfPath, ok := buscarPDFActaPorCodigo(acta.CodigoMesa)
	if !ok {
		return false, nil
	}

	rawPDF, err := p.realOCR.ProcesarPDF(pdfPath)
	if err != nil {
		return false, fmt.Errorf("procesar pdf referencia %s: %w", filepath.Base(pdfPath), err)
	}

	pdfActa := p.parser.ParseActaText(rawPDF, filepath.Base(pdfPath))
	if !actaTieneDatosPDF(pdfActa) {
		return false, fmt.Errorf("pdf referencia %s no devolvio votos suficientes", filepath.Base(pdfPath))
	}

	acta.Departamento = elegirTexto(pdfActa.Departamento, acta.Departamento)
	acta.Provincia = elegirTexto(pdfActa.Provincia, acta.Provincia)
	acta.Municipio = elegirTexto(pdfActa.Municipio, acta.Municipio)
	acta.Recinto = elegirTexto(pdfActa.Recinto, acta.Recinto)
	acta.Mesa = elegirTexto(pdfActa.Mesa, acta.Mesa)
	acta.CodigoMesa = elegirTexto(pdfActa.CodigoMesa, acta.CodigoMesa)
	acta.ActaID = elegirTexto(pdfActa.ActaID, acta.ActaID)
	acta.Candidatos = pdfActa.Candidatos
	acta.VotosValidos = pdfActa.VotosValidos
	acta.VotosBlancos = pdfActa.VotosBlancos
	acta.VotosNulos = pdfActa.VotosNulos
	acta.TotalVotos = pdfActa.TotalVotos
	acta.ElectoresHabilitados = pdfActa.ElectoresHabilitados
	acta.PapeletasAnfora = pdfActa.PapeletasAnfora
	acta.PapeletasNoUtilizadas = pdfActa.PapeletasNoUtilizadas

	return true, nil
}

func buscarPDFActaPorCodigo(codigo string) (string, bool) {
	name := "acta_" + codigo + ".pdf"
	cwd, _ := filepath.Abs(".")
	candidates := []string{
		filepath.Join(cwd, "pdf", name),
		filepath.Join(cwd, "RRV", "pdf", name),
		filepath.Join("pdf", name),
		filepath.Join("RRV", "pdf", name),
	}

	for _, path := range candidates {
		if info, err := os.Stat(path); err == nil && !info.IsDir() {
			return path, true
		}
	}
	return "", false
}

func actaTieneDatosPDF(acta *models.ActaRRV) bool {
	if acta == nil {
		return false
	}
	if acta.VotosValidos > 0 || acta.VotosBlancos > 0 || acta.VotosNulos > 0 {
		return true
	}
	for _, c := range acta.Candidatos {
		if c.Votos > 0 {
			return true
		}
	}
	return false
}

func elegirTexto(preferido, fallback string) string {
	if preferido != "" {
		return preferido
	}
	return fallback
}
