package service

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	_ "image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"rrv-backend/internal/models"
)

type voteROI struct {
	name   string
	x0, y0 float64
	x1, y1 float64
}

type voteProfile struct {
	name string
	rois []voteROI
}

var actaVoteProfiles = []voteProfile{
	{
		name: "acta_horizontal",
		rois: []voteROI{
			{name: "c1", x0: 0.325, y0: 0.290, x1: 0.395, y1: 0.335},
			{name: "c2", x0: 0.325, y0: 0.330, x1: 0.395, y1: 0.375},
			{name: "c3", x0: 0.325, y0: 0.370, x1: 0.395, y1: 0.415},
			{name: "c4", x0: 0.325, y0: 0.410, x1: 0.395, y1: 0.455},
			{name: "validos", x0: 0.325, y0: 0.670, x1: 0.395, y1: 0.715},
			{name: "blancos", x0: 0.325, y0: 0.720, x1: 0.395, y1: 0.765},
			{name: "nulos", x0: 0.325, y0: 0.765, x1: 0.395, y1: 0.810},
		},
	},
	{
		name: "acta_horizontal_derecha",
		rois: []voteROI{
			{name: "c1", x0: 0.340, y0: 0.285, x1: 0.410, y1: 0.335},
			{name: "c2", x0: 0.340, y0: 0.325, x1: 0.410, y1: 0.375},
			{name: "c3", x0: 0.340, y0: 0.365, x1: 0.410, y1: 0.415},
			{name: "c4", x0: 0.340, y0: 0.405, x1: 0.410, y1: 0.455},
			{name: "validos", x0: 0.340, y0: 0.670, x1: 0.410, y1: 0.715},
			{name: "blancos", x0: 0.340, y0: 0.720, x1: 0.410, y1: 0.765},
			{name: "nulos", x0: 0.340, y0: 0.765, x1: 0.410, y1: 0.810},
		},
	},
	{
		name: "acta_ejemplos_ancha",
		rois: []voteROI{
			{name: "c1", x0: 0.318, y0: 0.288, x1: 0.405, y1: 0.340},
			{name: "c2", x0: 0.318, y0: 0.326, x1: 0.405, y1: 0.378},
			{name: "c3", x0: 0.318, y0: 0.365, x1: 0.405, y1: 0.418},
			{name: "c4", x0: 0.318, y0: 0.404, x1: 0.405, y1: 0.458},
			{name: "validos", x0: 0.318, y0: 0.665, x1: 0.405, y1: 0.720},
			{name: "blancos", x0: 0.318, y0: 0.715, x1: 0.405, y1: 0.770},
			{name: "nulos", x0: 0.318, y0: 0.760, x1: 0.405, y1: 0.815},
		},
	},
}

type extractedVotes struct {
	Candidatos []models.Candidato
	Validos    int
	Blancos    int
	Nulos      int
	Score      int
	Profile    string
}

// ExtraerVotosPorCasillas lee solo las casillas numericas del acta con Tesseract
// configurado para digitos. Es un fallback para fotos de pantalla o camara donde
// el OCR de pagina completa no logra leer votos.
func (s *RealOCRService) ExtraerVotosPorCasillas(imagePath string) (*extractedVotes, error) {
	f, err := os.Open(imagePath)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	img, _, err := image.Decode(f)
	if err != nil {
		return nil, fmt.Errorf("decode image: %w", err)
	}

	var best *extractedVotes
	for _, profile := range actaVoteProfiles {
		got := &extractedVotes{Profile: profile.name}
		values := make(map[string]int)

		for _, roi := range profile.rois {
			value, ok := s.ocrVoteROI(img, roi)
			if !ok {
				continue
			}
			values[roi.name] = value
			got.Score++
		}

		if got.Score >= 4 {
			got.Candidatos = []models.Candidato{
				{CandidatoID: "C1", Votos: values["c1"]},
				{CandidatoID: "C2", Votos: values["c2"]},
				{CandidatoID: "C3", Votos: values["c3"]},
				{CandidatoID: "C4", Votos: values["c4"]},
			}
			got.Validos = values["validos"]
			got.Blancos = values["blancos"]
			got.Nulos = values["nulos"]
			sumaCand := 0
			for _, c := range got.Candidatos {
				sumaCand += c.Votos
			}
			if sumaCand > 0 && got.Validos != sumaCand {
				got.Validos = sumaCand
			}
		}

		if best == nil || got.Score > best.Score {
			best = got
		}
	}

	if best == nil || best.Score < 4 {
		return nil, fmt.Errorf("no se pudieron leer suficientes casillas de votos")
	}
	return best, nil
}

func (s *RealOCRService) ocrVoteROI(img image.Image, roi voteROI) (int, bool) {
	if value, ok := s.ocrVoteROIPorDigitos(img, roi); ok {
		return value, true
	}

	tmpPath, err := guardarROIAmplificada(img, roi)
	if err != nil {
		return 0, false
	}
	defer os.Remove(tmpPath)

	for _, psm := range []string{"7", "8", "13", "6"} {
		text, err := s.runDigitsOCR(tmpPath, psm)
		if err != nil {
			continue
		}

		value, ok := parseVoteDigits(text)
		if ok {
			return value, true
		}
	}

	return 0, false
}

func (s *RealOCRService) ocrVoteROIPorDigitos(img image.Image, roi voteROI) (int, bool) {
	var digits strings.Builder
	okCount := 0

	for i := 0; i < 3; i++ {
		cell := roi
		cellW := (roi.x1 - roi.x0) / 3
		cell.x0 = roi.x0 + float64(i)*cellW
		cell.x1 = cell.x0 + cellW

		// Reducir un poco cada celda evita capturar bordes negros de la cuadricula.
		padX := cellW * 0.12
		padY := (roi.y1 - roi.y0) * 0.12
		cell.x0 += padX
		cell.x1 -= padX
		cell.y0 += padY
		cell.y1 -= padY

		tmpPath, err := guardarROIAmplificada(img, cell)
		if err != nil {
			digits.WriteString("0")
			continue
		}

		digit, ok := s.ocrSingleDigit(tmpPath)
		os.Remove(tmpPath)
		if ok {
			digits.WriteString(digit)
			okCount++
		} else {
			digits.WriteString("0")
		}
	}

	if okCount < 2 {
		return 0, false
	}

	n, err := strconv.Atoi(digits.String())
	if err != nil || n < 0 || n > 999 {
		return 0, false
	}
	return n, true
}

func (s *RealOCRService) ocrSingleDigit(imagePath string) (string, bool) {
	for _, psm := range []string{"10", "13", "8"} {
		text, err := s.runDigitsOCR(imagePath, psm)
		if err != nil {
			continue
		}
		digit, ok := parseSingleDigit(text)
		if ok {
			return digit, true
		}
	}
	return "", false
}

func guardarROIAmplificada(img image.Image, roi voteROI) (string, error) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	rect := image.Rect(
		b.Min.X+int(roi.x0*float64(w)),
		b.Min.Y+int(roi.y0*float64(h)),
		b.Min.X+int(roi.x1*float64(w)),
		b.Min.Y+int(roi.y1*float64(h)),
	)
	rect = rect.Intersect(b)
	if rect.Empty() || rect.Dx() < 8 || rect.Dy() < 8 {
		return "", fmt.Errorf("roi vacia")
	}

	cropped := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
	draw.Draw(cropped, cropped.Bounds(), img, rect.Min, draw.Src)
	clean := aislarTintaAzul(cropped)

	scale := 5
	scaled := image.NewRGBA(image.Rect(0, 0, clean.Bounds().Dx()*scale, clean.Bounds().Dy()*scale))
	for y := 0; y < scaled.Bounds().Dy(); y++ {
		for x := 0; x < scaled.Bounds().Dx(); x++ {
			scaled.Set(x, y, clean.At(x/scale, y/scale))
		}
	}

	tmp, err := os.CreateTemp("", "vote_roi_*.jpg")
	if err != nil {
		return "", err
	}
	defer tmp.Close()

	if err := jpeg.Encode(tmp, scaled, &jpeg.Options{Quality: 96}); err != nil {
		os.Remove(tmp.Name())
		return "", err
	}
	return tmp.Name(), nil
}

func aislarTintaAzul(img image.Image) image.Image {
	b := img.Bounds()
	out := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))

	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			r8, g8, b8 := int(r>>8), int(g>>8), int(bl>>8)

			// La tinta de las actas es azul/morada. Los bordes negros de las
			// celdas y el texto impreso se blanquean para que OCR vea solo digitos.
			esAzul := b8 > r8+18 && b8 > g8+6 && b8 > 70
			esMorado := b8 > r8+8 && r8 > g8+5 && b8 > 80
			esTrazoOscuroAzulado := b8 > g8+10 && r8 < 150 && g8 < 150

			if esAzul || esMorado || esTrazoOscuroAzulado {
				out.Set(x-b.Min.X, y-b.Min.Y, color.Black)
			} else {
				out.Set(x-b.Min.X, y-b.Min.Y, color.White)
			}
		}
	}

	return out
}

func (s *RealOCRService) runDigitsOCR(imagePath, psm string) (string, error) {
	tmpFile, err := os.CreateTemp("", "ocr_digits_*")
	if err != nil {
		return "", err
	}
	tmpFile.Close()
	outputBase := tmpFile.Name()
	os.Remove(outputBase)

	absDir, _ := filepath.Abs(".")
	tessdataDir := filepath.Join(absDir, "tessdata")

	cmd := exec.Command(s.tesseractPath,
		imagePath,
		outputBase,
		"--tessdata-dir", tessdataDir,
		"-l", s.language,
		"--psm", psm,
		"--oem", "1",
		"-c", "tessedit_char_whitelist=0123456789OoIl|SsBbZzGg",
	)

	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("tesseract digits fallo: %v | %s", err, string(output))
	}

	textPath := outputBase + ".txt"
	textBytes, err := os.ReadFile(textPath)
	os.Remove(textPath)
	if err != nil {
		return "", err
	}
	return string(textBytes), nil
}

func parseVoteDigits(text string) (int, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return 0, false
	}

	text = strings.NewReplacer(
		"O", "0", "o", "0",
		"I", "1", "l", "1", "|", "1",
		"S", "5", "s", "5",
		"B", "8", "b", "6",
		"Z", "2", "z", "2",
		"G", "6", "g", "9",
	).Replace(text)

	re := regexp.MustCompile(`\d`)
	digits := strings.Join(re.FindAllString(text, -1), "")
	if len(digits) < 1 {
		return 0, false
	}
	if len(digits) > 3 {
		digits = digits[len(digits)-3:]
	}
	n, err := strconv.Atoi(digits)
	if err != nil || n < 0 || n > 999 {
		return 0, false
	}
	return n, true
}

func parseSingleDigit(text string) (string, bool) {
	text = strings.TrimSpace(text)
	if text == "" {
		return "", false
	}

	text = strings.NewReplacer(
		"O", "0", "o", "0", "Q", "0",
		"I", "1", "l", "1", "|", "1", "i", "1", "!", "1",
		"Z", "2", "z", "2",
		"E", "3",
		"A", "4",
		"S", "5", "s", "5",
		"G", "6",
		"T", "7",
		"B", "8",
		"g", "9", "q", "9",
	).Replace(text)

	re := regexp.MustCompile(`\d`)
	digits := re.FindAllString(text, -1)
	if len(digits) == 0 {
		return "", false
	}
	return digits[len(digits)-1], true
}
