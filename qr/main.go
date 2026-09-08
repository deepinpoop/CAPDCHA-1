package main

import (
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
)

const unsplashAccessKey = "ajjFLji7SSrb6iQ01Et3Z9iHFq9CmSosMQkl1lK3Ha4"

func nextBackgroundHandler(w http.ResponseWriter, r *http.Request) {
	url := "https://api.unsplash.com/photos/random?query=nature&orientation=landscape"
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		http.Error(w, "failed to create request", http.StatusInternalServerError)
		return
	}
	req.Header.Set("Authorization", "Client-ID "+unsplashAccessKey)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		http.Error(w, "failed to fetch from unsplash", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		http.Error(w, "unsplash returned error", http.StatusBadGateway)
		return
	}

	contentType := resp.Header.Get("Content-Type")
	if !strings.HasPrefix(contentType, "image/") {
		http.Error(w, "unexpected content type", http.StatusBadGateway)
		return
	}

	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Cache-Control", "public, max-age=86400")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	io.Copy(w, resp.Body)
}

func main() {
	http.HandleFunc("/api/next-background", nextBackgroundHandler)
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		http.ServeFile(w, r, "index.html")
	})

	fmt.Println("Server running on http://localhost:3001")
	log.Fatal(http.ListenAndServe(":3001", nil))
}
