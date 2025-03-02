package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/bootdotdev/learn-file-storage-s3-golang-starter/internal/database"

	"github.com/joho/godotenv"
	_ "github.com/lib/pq"
)

type apiConfig struct {
	db               database.Client
	jwtSecret        string
	platform         string
	filepathRoot     string
	assetsRoot       string
	s3Client         *s3.Client
	s3Bucket         string
	s3Region         string
	s3CfDistribution string
	port             string
}

func main() {
	godotenv.Load(".env")

	pathToDB := os.Getenv("DB_PATH")
	if pathToDB == "" {
		log.Fatal("DB_URL must be set")
	}

	db, err := database.NewClient(pathToDB)
	if err != nil {
		log.Fatalf("Couldn't connect to database: %v", err)
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Fatal("JWT_SECRET environment variable is not set")
	}

	platform := os.Getenv("PLATFORM")
	if platform == "" {
		log.Fatal("PLATFORM environment variable is not set")
	}

	filepathRoot := os.Getenv("FILEPATH_ROOT")
	if filepathRoot == "" {
		log.Fatal("FILEPATH_ROOT environment variable is not set")
	}

	assetsRoot := os.Getenv("ASSETS_ROOT")
	if assetsRoot == "" {
		log.Fatal("ASSETS_ROOT environment variable is not set")
	}

	s3Bucket := os.Getenv("S3_BUCKET")
	if s3Bucket == "" {
		log.Fatal("S3_BUCKET environment variable is not set")
	}

	s3Region := os.Getenv("S3_REGION")
	if s3Region == "" {
		log.Fatal("S3_REGION environment variable is not set")
	}

	s3CfDistribution := os.Getenv("S3_CF_DISTRO")
	if s3CfDistribution == "" {
		log.Fatal("S3_CF_DISTRO environment variable is not set")
	}

	port := os.Getenv("PORT")
	if port == "" {
		log.Fatal("PORT environment variable is not set")
	}

	awsConfig, err := config.LoadDefaultConfig(context.Background(), config.WithRegion(s3Region))
	if err != nil {
		log.Fatal("Failed to load default aws config")
	}

	s3Client := s3.NewFromConfig(awsConfig)

	cfg := apiConfig{
		db:               db,
		jwtSecret:        jwtSecret,
		platform:         platform,
		filepathRoot:     filepathRoot,
		assetsRoot:       assetsRoot,
		s3Client:         s3Client,
		s3Bucket:         s3Bucket,
		s3Region:         s3Region,
		s3CfDistribution: s3CfDistribution,
		port:             port,
	}

	err = cfg.ensureAssetsDir()
	if err != nil {
		log.Fatalf("Couldn't create assets directory: %v", err)
	}

	mux := http.NewServeMux()
	appHandler := http.StripPrefix("/app", http.FileServer(http.Dir(filepathRoot)))
	mux.Handle("/app/", appHandler)

	assetsHandler := http.StripPrefix("/assets", http.FileServer(http.Dir(assetsRoot)))
	mux.Handle("/assets/", noCacheMiddleware(assetsHandler))

	mux.HandleFunc("POST /api/login", cfg.handlerLogin)
	mux.HandleFunc("POST /api/refresh", cfg.handlerRefresh)
	mux.HandleFunc("POST /api/revoke", cfg.handlerRevoke)

	mux.HandleFunc("POST /api/users", cfg.handlerUsersCreate)

	mux.HandleFunc("POST /api/videos", cfg.handlerVideoMetaCreate)
	mux.HandleFunc("POST /api/thumbnail_upload/{videoID}", cfg.handlerUploadThumbnail)
	mux.HandleFunc("POST /api/video_upload/{videoID}", cfg.handlerUploadVideo)
	mux.HandleFunc("GET /api/videos", cfg.handlerVideosRetrieve)
	mux.HandleFunc("GET /api/videos/{videoID}", cfg.handlerVideoGet)
	mux.HandleFunc("DELETE /api/videos/{videoID}", cfg.handlerVideoMetaDelete)

	mux.HandleFunc("POST /admin/reset", cfg.handlerReset)

	srv := &http.Server{
		Addr:    ":" + port,
		Handler: mux,
	}

	log.Printf("Serving on: http://localhost:%s/app/\n", port)
	log.Fatal(srv.ListenAndServe())
}

type FFProbeOutput struct {
	Streams []struct {
		DisplayAspectRatio string `json:"display_aspect_ratio"`
	} `json:"streams"`
}

func getVideoAspectRatio(filePath string) (string, error) {
	command := exec.Command("ffprobe", "-v", "error", "-print_format", "json", "-show_streams", filePath)
	var stdout bytes.Buffer
	command.Stdout = &stdout
	if err := command.Run(); err != nil {
		log.Print("Error: Failed to execute ffprobe command in terminal")
		return "", err
	}

	var data FFProbeOutput
	if err := json.Unmarshal(stdout.Bytes(), &data); err != nil {
		log.Print("Error: Failed to unmarshal json from the ffprobe command")
		return "", err
	}

	if len(data.Streams) == 0 {
		return "", fmt.Errorf("no streams found in video file")
	}

	switch data.Streams[0].DisplayAspectRatio {
	case "16:9":
		return "16:9", nil
	case "9:16":
		return "9:16", nil
	default:
		return "other", nil
	}
}

func processVideoForFastStart(filePath string) (string, error) {
	outputFilePath := filePath + ".processing"

	command := exec.Command("ffmpeg", "-i", filePath, "-c", "copy", "-movflags", "faststart", "-f", "mp4", outputFilePath)
	err := command.Run()
	if err != nil {
		return "", err
	}

	return outputFilePath, nil
}
