package handlers

import (
	"encoding/json"
	"fmt"
	"log"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/jmoiron/sqlx"
	"go.opentelemetry.io/otel"
    "go.opentelemetry.io/otel/attribute"

	"sample-backend/internal/models"
)

type SearchHandler struct {
	db *sqlx.DB
}

func NewSearchHandler(db *sqlx.DB) *SearchHandler {
	return &SearchHandler{db: db}
}

func (h *SearchHandler) SearchProducts(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	log.Printf("[API] Search products request from %s", r.RemoteAddr)

    tracer := otel.Tracer("product-search-backend")
    _, span := tracer.Start(r.Context(), "search_products")
    defer span.End()

	setJSONHeaders(w)

	if r.Method != "POST" {
		log.Printf("[ERROR] Invalid method: %s", r.Method)
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var searchReq models.SearchRequest
	if err := json.NewDecoder(r.Body).Decode(&searchReq); err != nil {
		log.Printf("[ERROR] Failed to decode request body: %v", err)
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

    span.SetAttributes(
        attribute.String("search.column", searchReq.Column),
        attribute.String("search.keyword", searchReq.Keyword),
        attribute.Int("search.page", searchReq.Page),
        attribute.Int("search.limit", searchReq.Limit),
    )

	log.Printf("[API] Search request - column: %s, keyword: %s, page: %d, limit: %d",
		searchReq.Column, searchReq.Keyword, searchReq.Page, searchReq.Limit)

	// バリデーション
	validColumns := map[string]bool{
		"name":        true,
		"category":    true,
		"brand":       true,
		"model":       true,
		"description": true,
	}
	if !validColumns[searchReq.Column] {
		log.Printf("[ERROR] Invalid search column: %s", searchReq.Column)
		http.Error(w, "Invalid search column", http.StatusBadRequest)
		return
	}

	if searchReq.Page < 1 {
		searchReq.Page = 1
	}
	if searchReq.Limit < 1 || searchReq.Limit > 100 {
		searchReq.Limit = 10
	}

	offset := (searchReq.Page - 1) * searchReq.Limit
	log.Printf("[API] Validated params - page: %d, limit: %d, offset: %d", searchReq.Page, searchReq.Limit, offset)

	// 検索条件
	searchTerm := "%" + strings.TrimSpace(searchReq.Keyword) + "%"
	log.Printf("[DB] Search term: %s", searchTerm)

	// 総件数を取得
	log.Println("[DB] Executing search count query...")
	var totalCount int
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM products WHERE %s LIKE ?", searchReq.Column)
	err := h.db.Get(&totalCount, countQuery, searchTerm)
	if err != nil {
		log.Printf("[DB ERROR] Failed to get search count: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	log.Printf("[DB] Search result count: %d", totalCount)

	// 検索結果を取得
	log.Printf("[DB] Executing search query with limit: %d, offset: %d", searchReq.Limit, offset)
	products := []models.Product{}
	searchQuery := fmt.Sprintf("SELECT id, name, category, brand, model, description, price, created_at FROM products WHERE %s LIKE ? ORDER BY id LIMIT ? OFFSET ?", searchReq.Column)
	err = h.db.Select(&products, searchQuery, searchTerm, searchReq.Limit, offset)
	if err != nil {
		log.Printf("[DB ERROR] Failed to execute search query: %v", err)
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	log.Printf("[DB] Retrieved %d search results", len(products))

    span.SetAttributes(
        attribute.Int("search.total_count", totalCount),
        attribute.Int("search.returned_count", len(products)),
    )

	totalPages := int(math.Ceil(float64(totalCount) / float64(searchReq.Limit)))
	log.Printf("[API] Calculated total pages: %d", totalPages)

	response := models.PaginatedResponse{
		Products:   products,
		Page:       searchReq.Page,
		Limit:      searchReq.Limit,
		TotalPages: totalPages,
		Count:      totalCount,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("[ERROR] Failed to encode search response: %v", err)
		return
	}

	duration := time.Since(start)
	log.Printf("[API] Search completed in %v - found %d products", duration, len(products))
}
