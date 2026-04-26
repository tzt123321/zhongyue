package search

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"zhongyue_refactored/internal/model"
)

type ESClient struct {
	baseURL    string
	httpClient *http.Client
}

func NewESClient(baseURL string) *ESClient {
	return &ESClient{
		baseURL: baseURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// CreateTrackIndex creates the tracks index with mappings
func (c *ESClient) CreateTrackIndex(ctx context.Context) error {
	mapping := map[string]interface{}{
		"mappings": map[string]interface{}{
			"properties": map[string]interface{}{
				"id":          map[string]string{"type": "long"},
				"title":       map[string]string{"type": "text", "analyzer": "standard"},
				"artist_name": map[string]string{"type": "text", "analyzer": "standard"},
				"album_name":  map[string]string{"type": "text", "analyzer": "standard"},
				"genre":       map[string]string{"type": "keyword"},
				"duration":    map[string]string{"type": "integer"},
				"play_count":  map[string]string{"type": "integer"},
				"created_at":  map[string]string{"type": "date"},
			},
		},
	}
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(mapping)
	req, _ := http.NewRequestWithContext(ctx, "PUT", c.baseURL+"/tracks", &buf)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 400 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ES create index error: %s", string(body))
	}
	return nil
}

// IndexTrack indexes a single track document
func (c *ESClient) IndexTrack(ctx context.Context, track *model.Track) error {
	doc := map[string]interface{}{
		"id":         track.ID,
		"title":      track.Title,
		"play_count": track.PlayCount,
		"created_at": track.CreatedAt,
	}
	if track.Artist != nil {
		doc["artist_name"] = track.Artist.Name
	}
	if track.Album != nil {
		doc["album_name"] = track.Album.Name
	}
	if track.Duration.Valid {
		doc["duration"] = int(track.Duration.Int32)
	}

	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(doc)
	url := fmt.Sprintf("%s/tracks/_doc/%d", c.baseURL, track.ID)
	req, _ := http.NewRequestWithContext(ctx, "PUT", url, &buf)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 && resp.StatusCode != 201 {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("ES index error: %s", string(body))
	}
	return nil
}

// Search performs a multi_match search across tracks and returns ordered IDs
func (c *ESClient) Search(ctx context.Context, query string, size int) ([]int64, error) {
	body := map[string]interface{}{
		"query": map[string]interface{}{
			"multi_match": map[string]interface{}{
				"query":  query,
				"fields": []string{"title^3", "artist_name^2", "album_name"},
				"type":   "best_fields",
			},
		},
		"size": size,
	}
	var buf bytes.Buffer
	json.NewEncoder(&buf).Encode(body)
	req, _ := http.NewRequestWithContext(ctx, "POST", c.baseURL+"/tracks/_search", &buf)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("ES search error: %s", string(body))
	}
	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)
	hits, ok := result["hits"].(map[string]interface{})["hits"].([]interface{})
	if !ok || len(hits) == 0 {
		return nil, nil
	}
	ids := make([]int64, 0, len(hits))
	for _, hit := range hits {
		id := int64(hit.(map[string]interface{})["_id"].(float64))
		ids = append(ids, id)
	}
	return ids, nil
}