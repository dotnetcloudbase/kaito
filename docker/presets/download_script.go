/*
 * This script allows you to download llama and falcon weights using parallel wget.
 * Llama is to be downloaded via hosted web server.
 * Falcon from public url.
 */
package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
)

const (
	PublicLink  = "public"
	PrivateLink = "private"
)

func getFilenameFromURL(url string) string {
	return filepath.Base(url)
}

func downloadFile(folderPath string, url string, token string, wg *sync.WaitGroup) {
	defer wg.Done()

	fileName := getFilenameFromURL(url)
	fp := filepath.Join(folderPath, fileName)

	// Create the file
	out, err := os.Create(fp)
	if err != nil {
		log.Fatal(err)
	}
	defer out.Close()

	// Create new request
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		log.Fatal(err)
	}
	// If token is provided, add to request header
	if token != "" {
		req.Header.Add("Authorization", "Bearer "+token)
	}

	// Execute the request
	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		log.Fatal(err)
	}
	defer resp.Body.Close()

	// Track progress
	totalBytes := resp.ContentLength
	var bytesRead int64

	progressReader := io.TeeReader(resp.Body, &WriteCounter{filename: fp, total: totalBytes, read: &bytesRead})

	// Write the data to the file
	_, err = io.Copy(out, progressReader)
	if err != nil {
		log.Fatal(err)
	}
}

type WriteCounter struct {
	filename     string
	total        int64
	read         *int64
	lastReported int64
}

func (wc *WriteCounter) Write(p []byte) (int, error) {
	n := len(p)
	*wc.read += int64(n)

	// Calculate every 1% increment of the total size
	onePercent := wc.total / 100

	// Check if the bytes read has surpassed another 1% increment since the last reported value
	if *wc.read-wc.lastReported >= onePercent {
		fmt.Printf("\rDownloading [%s]: %d out of %d bytes (%.2f%%)\n", filepath.Base(wc.filename), *wc.read, wc.total, float64(*wc.read)/float64(wc.total)*100)
		wc.lastReported = *wc.read
	}

	return n, nil
}

func falconCommonURLs(modelVersion string) []string {
	return []string{
		fmt.Sprintf("https://huggingface.co/%s/raw/main/config.json", modelVersion),
		fmt.Sprintf("https://huggingface.co/%s/raw/main/pytorch_model.bin.index.json", modelVersion),
		fmt.Sprintf("https://huggingface.co/%s/raw/main/tokenizer.json", modelVersion),
		fmt.Sprintf("https://huggingface.co/%s/raw/main/tokenizer_config.json", modelVersion),
		fmt.Sprintf("https://huggingface.co/%s/raw/main/special_tokens_map.json", modelVersion),
		fmt.Sprintf("https://huggingface.co/%s/raw/main/configuration_falcon.py", modelVersion),
		fmt.Sprintf("https://huggingface.co/%s/raw/main/generation_config.json", modelVersion),
		fmt.Sprintf("https://huggingface.co/%s/raw/main/modeling_falcon.py", modelVersion),
	}
}

func falconModelURLs(modelVersion string, count int) (urls []string) {
	for i := 1; i <= count; i++ {
		url := fmt.Sprintf("https://huggingface.co/%s/resolve/main/pytorch_model-%05d-of-%05d.bin", modelVersion, i, count)
		urls = append(urls, url)
	}
	return
}

func getURLsForModel(linkType, baseURL, modelVersion string) []string {
	if linkType == PublicLink {
		switch modelVersion {
		case "tiiuae/falcon-7b", "tiiuae/falcon-7b-instruct":
			return append(falconModelURLs(modelVersion, 2), falconCommonURLs(modelVersion)...)
		case "tiiuae/falcon-40b", "tiiuae/falcon-40b-instruct":
			return append(falconModelURLs(modelVersion, 9), falconCommonURLs(modelVersion)...)
		default:
			log.Fatalf("Invalid model version for public link: %s", modelVersion)
			return nil
		}
	} else {
		return getPrivateURLsForModel(baseURL, modelVersion)
	}
}

func getPrivateURLsForModel(baseURL, modelVersion string) []string {
	switch modelVersion {
	case "llama-2-7b", "llama-2-7b-chat":
		return []string{
			fmt.Sprintf("%s%s/consolidated.00.pth", baseURL, modelVersion),
			fmt.Sprintf("%s%s/params.json", baseURL, modelVersion),
		}
	case "llama-2-13b", "llama-2-13b-chat":
		return []string{
			fmt.Sprintf("%s%s/consolidated.00.pth", baseURL, modelVersion),
			fmt.Sprintf("%s%s/consolidated.01.pth", baseURL, modelVersion),
			fmt.Sprintf("%s%s/params.json", baseURL, modelVersion),
		}

	case "llama-2-70b", "llama-2-70b-chat":
		return []string{
			fmt.Sprintf("%s%s/consolidated.00.pth", baseURL, modelVersion),
			fmt.Sprintf("%s%s/consolidated.01.pth", baseURL, modelVersion),
			fmt.Sprintf("%s%s/consolidated.03.pth", baseURL, modelVersion),
			fmt.Sprintf("%s%s/consolidated.04.pth", baseURL, modelVersion),
			fmt.Sprintf("%s%s/consolidated.05.pth", baseURL, modelVersion),
			fmt.Sprintf("%s%s/consolidated.06.pth", baseURL, modelVersion),
			fmt.Sprintf("%s%s/consolidated.07.pth", baseURL, modelVersion),
			fmt.Sprintf("%s%s/params.json", baseURL, modelVersion),
		}

	default:
		log.Fatalf("Invalid model version for private link: %s", modelVersion)
		return nil
	}
}

func ensureDirExists(dirName string) {
	if _, err := os.Stat(dirName); os.IsNotExist(err) {
		err := os.MkdirAll(dirName, 0755)
		if err != nil {
			log.Fatalf("Failed to create directory: %v", err)
		}
	}
}

func main() {
	if len(os.Args) < 4 {
		log.Fatalf("Usage: %s <link_type> <model_version> <output_directory> [external_IP] [external_port]", os.Args[0])
	}

	linkType := os.Args[1]
	modelVersion := os.Args[2]
	outputDirectory := os.Args[3]
	ensureDirExists(outputDirectory)

	token := ""
	baseURL := ""
	if linkType == PrivateLink {
		if len(os.Args) != 6 {
			log.Fatalf("Usage (private link): %s <link_type> <model_version> <output_directory> <external_IP> <external_port>", os.Args[0])
		}
		token = os.Getenv("AUTH_TOKEN_ENV_VAR")
		if token == "" {
			log.Fatal("AUTH_TOKEN_ENV_VAR not set!")
		}
		externalIP := os.Args[4]
		externalPort := os.Args[5]
		baseURL = "http://" + externalIP + ":" + externalPort + "/download/"
	}

	urls := getURLsForModel(linkType, baseURL, modelVersion)
	var wg sync.WaitGroup

	for _, url := range urls {
		wg.Add(1)
		go downloadFile(outputDirectory, url, token, &wg)
	}

	wg.Wait()
}