package main

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/auth"
	"github.com/google/uuid"
)

func (cfg *apiConfig) handlerUploadThumbnail(w http.ResponseWriter, r *http.Request) {
	videoIDString := r.PathValue("videoID")
	videoID, err := uuid.Parse(videoIDString)
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Invalid ID", err)
		return
	}

	token, err := auth.GetBearerToken(r.Header)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't find JWT", err)
		return
	}

	userID, err := auth.ValidateJWT(token, cfg.jwtSecret)
	if err != nil {
		respondWithError(w, http.StatusUnauthorized, "Couldn't validate JWT", err)
		return
	}

	fmt.Println("uploading thumbnail for video", videoID, "by user", userID)

	const maxMemory = 10 << 20
	r.ParseMultipartForm(maxMemory)
	file, header, err := r.FormFile("thumbnail")
	if err != nil {
		respondWithError(w, http.StatusBadRequest, "Unable to parse form file", err)
		return
	}
	defer file.Close()

	// Get the media type and file data from the form data
	initMediaType := header.Header.Get("Content-Type")
	mediaType, _, err := mime.ParseMediaType(initMediaType)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error parsing the media type from the content header", err)
		return
	}
	if mediaType != "image/jpeg" && mediaType != "image/png" {
		respondWithError(w, http.StatusInternalServerError, "Error: The media type uploaded is invalid", err)
		return
	}

	// Fetch the video to update
	video, err := cfg.db.GetVideo(videoID)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Unable to get video", err)
		return
	}
	if video.UserID != userID {
		respondWithError(w, http.StatusUnauthorized, "You are not authorized to upload a thumbnail for this video", nil)
		return
	}

	fileExtension := ""
	if strings.Contains(mediaType, "/") {
		fileExtension = strings.Split(mediaType, "/")[1]
	}

	randomBytes := make([]byte, 4)
	if _, err := rand.Read(randomBytes); err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error generating random bytes", err)
		return
	}

	randomUrl := base64.RawURLEncoding.EncodeToString(randomBytes)

	thumbnailUrl := filepath.Join(cfg.assetsRoot, randomUrl+"."+fileExtension)

	formattedFile, err := os.Create(thumbnailUrl)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "Error occured while creating the file", err)
		return
	}
	io.Copy(formattedFile, file)

	video.ThumbnailURL = &thumbnailUrl

	err = cfg.db.UpdateVideo(video)
	if err != nil {
		respondWithError(w, http.StatusInternalServerError, "There was an error updating the video", err)
		return
	}

	respondWithJSON(w, http.StatusOK, video)
}
