package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"golang.org/x/image/draw"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/canvas"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/theme"
)

type Post struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

type RedditResponse struct {
	Data struct {
		After    string `json:"after"`
		Children []struct {
			Data Post `json:"data"`
		} `json:"children"`
	} `json:"data"`
}

func fetchRedditData(subreddit string, limit int) ([]Post, error) {
	var allPosts []Post
	after := ""
	for {
		url := fmt.Sprintf("https://www.reddit.com/r/%s/.json?limit=%d&after=%s", subreddit, limit, after)
		log.Println("Fetching URL:", url)
		req, _ := http.NewRequest("GET", url, nil)
		req.Header.Set("User-Agent", "Go-Reddit-Client")

		client := &http.Client{}
		resp, err := client.Do(req)
		if err != nil {
			log.Fatalf("Error making HTTP request: %v", err)
			return nil, err
		}
		defer resp.Body.Close()

		body, err := io.ReadAll(resp.Body)
		if err != nil {
			log.Fatalf("Error reading response body: %v", err)
			return nil, err
		}

		var redditResponse RedditResponse
		err = json.Unmarshal(body, &redditResponse)
		if err != nil {
			log.Fatalf("Error unmarshalling JSON: %v", err)
			return nil, err
		}

		for _, child := range redditResponse.Data.Children {
			allPosts = append(allPosts, child.Data)
		}

		if len(allPosts) >= limit || redditResponse.Data.After == "" {
			break
		}

		after = redditResponse.Data.After
	}

	log.Printf("Fetched %d posts", len(allPosts))
	return allPosts[:limit], nil
}

func isValidImageURL(url string) bool {
	re := regexp.MustCompile(`\.(gif|jpeg|jpg|png)$`)
	return re.MatchString(strings.ToLower(url))
}

func downloadImage(url string) (image.Image, error) {
	resp, err := http.Get(url)
	if err != nil {
		return nil, fmt.Errorf("failed to download image: %w", err)
	}
	defer resp.Body.Close()

	img, format, err := image.Decode(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}
	log.Printf("Image format: %s", format)
	return img, nil
}

func saveImageToFile(img image.Image, fileName string) error {
	dir := "imgDls"
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	path := filepath.Join(dir, fileName)
	file, err := os.Create(path)
	if err != nil {
		return fmt.Errorf("failed to create file: %w", err)
	}
	defer file.Close()

	switch filepath.Ext(path) {
	case ".jpg", ".jpeg":
		err = jpeg.Encode(file, img, nil)
	case ".png":
		err = png.Encode(file, img)
	case ".gif":
		err = gif.Encode(file, img, nil)
	default:
		return fmt.Errorf("unsupported file extension: %s", filepath.Ext(path))
	}
	if err != nil {
		return fmt.Errorf("failed to encode image: %w", err)
	}
	return nil
}

func resizeImage(img image.Image, maxWidth int) image.Image {
	bounds := img.Bounds()
	width := bounds.Dx()
	height := bounds.Dy()

	if width > maxWidth {
		ratio := float64(maxWidth) / float64(width)
		newWidth := int(float64(width) * ratio)
		newHeight := int(float64(height) * ratio)

		newImage := image.NewRGBA(image.Rect(0, 0, newWidth, newHeight))
		draw.CatmullRom.Scale(newImage, newImage.Bounds(), img, bounds, draw.Over, nil)
		return newImage
	}

	return img
}

func main() {
	subreddit := flag.String("subreddit", "archlinux", "Name of the subreddit to fetch images from")
	download := flag.Bool("download", false, "Download images to the current directory when true")
	limit := flag.Int("limit", 25, "Number of posts to fetch")
	flag.Parse()

	a := app.New()
	w := a.NewWindow("Reddit Image Feed")

	log.Println("Fetching data from subreddit:", *subreddit)
	posts, err := fetchRedditData(*subreddit, *limit)
	if err != nil {
		log.Fatalf("Error fetching data: %v", err)
		return
	}

	content := container.NewVBox()

	for _, post := range posts {
		if isValidImageURL(post.URL) {
			img, err := downloadImage(post.URL)
			if err != nil {
				log.Printf("Skipping post: %s - %s. Error: %v", post.Title, post.URL, err)
				continue
			}

			img = resizeImage(img, 400)

			image := canvas.NewImageFromImage(img)
			image.FillMode = canvas.ImageFillOriginal

			title := canvas.NewText(post.Title, theme.ForegroundColor())
			title.TextStyle = fyne.TextStyle{Bold: true}
			title.TextSize = 16

			content.Add(title)
			content.Add(image)

			if *download {
				fileName := fmt.Sprintf("%s%s", strings.ReplaceAll(post.Title, " ", "_"), filepath.Ext(post.URL))
				filePath := filepath.Join(".", fileName)
				err := saveImageToFile(img, filePath)
				if err != nil {
					log.Printf("Failed to save image: %v", err)
				} else {
					log.Printf("Saved image: %s", filePath)
				}
			}
		} else {
			log.Printf("Skipping non-image URL: %s", post.URL)
		}
	}

	scroll := container.NewScroll(content)
	w.SetContent(scroll)
	w.Resize(fyne.NewSize(800, 600))
	w.ShowAndRun()
}
