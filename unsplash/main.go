package main

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

const unsplashAccessKey = "cjjFLji7SSrb6iQ01Et3Z9iHFq9CmSosMQkl1lK3Ha4"

const (
	cacheDir     = "img"
	cacheCount   = 10
	rateLimitErr = "Rate Limit Exceeded"
)

type unsplashPhoto struct {
	Urls struct {
		Regular string `json:"regular"`
	} `json:"urls"`
}

var (
	cacheMu     sync.Mutex
	cacheSlot   int
	serveOffset int
)

func cachePath(i int) string {
	return filepath.Join(cacheDir, fmt.Sprintf("bg_%d.jpg", i))
}

func cachedImagesExist() bool {
	for i := 0; i < cacheCount; i++ {
		if _, err := os.Stat(cachePath(i)); err != nil {
			return false
		}
	}
	return true
}

func saveToCache(data []byte, contentType string) {
	cacheMu.Lock()
	defer cacheMu.Unlock()

	ext := ".jpg"
	if strings.Contains(contentType, "png") {
		ext = ".png"
	}
	path := filepath.Join(cacheDir, fmt.Sprintf("bg_%d%s", cacheSlot, ext))
	if err := os.WriteFile(path, data, 0o644); err == nil {
		cacheSlot = (cacheSlot + 1) % cacheCount
	}
}

func serveNextCached(w http.ResponseWriter) bool {
	cacheMu.Lock()
	defer cacheMu.Unlock()

	for i := 0; i < cacheCount; i++ {
		idx := (serveOffset + i) % cacheCount
		data, err := os.ReadFile(cachePath(idx))
		if err != nil {
			continue
		}
		serveOffset = idx + 1
		w.Header().Set("Content-Type", "image/jpeg")
		w.Header().Set("Cache-Control", "public, max-age=86400")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Write(data)
		return true
	}
	return false
}

func nextBackgroundHandler(w http.ResponseWriter, r *http.Request) {
	apiURL := "https://api.unsplash.com/photos/random?query=nature&orientation=landscape"
	req, err := http.NewRequest("GET", apiURL, nil)
	if err != nil {
		http.Error(w, "failed to create request", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Authorization", "Client-ID "+unsplashAccessKey)

	client := &http.Client{}
	metaResp, err := client.Do(req)
	if err != nil {
		if !serveNextCached(w) {
			http.Error(w, "failed to fetch from unsplash", http.StatusBadGateway)
		}
		return
	}
	defer metaResp.Body.Close()

	if metaResp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(metaResp.Body)
		if strings.Contains(string(body), rateLimitErr) || metaResp.StatusCode == http.StatusTooManyRequests {
			if serveNextCached(w) {
				return
			}
		}
		http.Error(w, fmt.Sprintf("unsplash error %d: %s", metaResp.StatusCode, body), http.StatusBadGateway)
		return
	}

	var photo unsplashPhoto
	if err := json.NewDecoder(metaResp.Body).Decode(&photo); err != nil {
		http.Error(w, "failed to parse unsplash response", http.StatusBadGateway)
		return
	}

	if photo.Urls.Regular == "" {
		http.Error(w, "no image url in response", http.StatusBadGateway)
		return
	}

	imgResp, err := client.Get(photo.Urls.Regular)
	if err != nil {
		http.Error(w, "failed to download image", http.StatusBadGateway)
		return
	}
	defer imgResp.Body.Close()

	if imgResp.StatusCode != http.StatusOK {
		http.Error(w, "image download failed", http.StatusBadGateway)
		return
	}

	contentType := imgResp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		http.Error(w, "unexpected content type", http.StatusBadGateway)
		return
	}

	imgData, err := io.ReadAll(imgResp.Body)
	if err != nil {
		http.Error(w, "failed to read image", http.StatusBadGateway)
		return
	}

	saveToCache(imgData, contentType)

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	w.Write(imgData)
}

func initCachedImages() {
	if cachedImagesExist() {
		log.Printf("using %s cached images", cacheDir)
	}
}

func main() {
	http.HandleFunc("/api/next-background", nextBackgroundHandler)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})

	fmt.Println("Server running on http://localhost:3002")
	log.Fatal(http.ListenAndServe(":3002", nil))
}
