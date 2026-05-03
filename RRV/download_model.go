package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
)

func main() {
	err := os.MkdirAll("tessdata", 0755)
	if err != nil {
		panic(err)
	}

	url := "https://github.com/tesseract-ocr/tessdata_fast/raw/main/spa.traineddata"
	fmt.Printf("Downloading %s...\n", url)

	resp, err := http.Get(url)
	if err != nil {
		panic(err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		panic(fmt.Sprintf("bad status: %s", resp.Status))
	}

	out, err := os.Create("tessdata/spa.traineddata")
	if err != nil {
		panic(err)
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		panic(err)
	}

	fmt.Println("Download complete!")
}
