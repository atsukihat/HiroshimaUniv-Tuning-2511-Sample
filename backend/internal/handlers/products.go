package handlers

import (
	"encoding/json"
	"log"
	"math"
	"net/http"
	"strconv"
	"time"

	"github.com/jmoiron/sqlx"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"

	"sample-backend/internal/models"
)

type ProductHandler struct {
	db *sqlx.DB
}

func NewProductHandler(db *sqlx.DB) *ProductHandler {
	return &ProductHandler{db: db}
}

func (h *ProductHandler) GetProducts(w http.ResponseWriter, r *http.Request) {
	start := time.Now()
	log.Printf("[API] Get products request from %s", r.RemoteAddr)

	// トレースの開始
	tracer := otel.Tracer("product-search-backend")
	_, span := tracer.Start(r.Context(), "get_products")
	defer span.End()

	setJSONHeaders(w)

	// ページネーションパラメータの取得
	pageStr := r.URL.Query().Get("page")
	limitStr := r.URL.Query().Get("limit")
	log.Printf("[API] Request params - page: %s, limit: %s", pageStr, limitStr)

	page, err := strconv.Atoi(pageStr)
	if err != nil || page < 1 {
		page = 1
	}

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < 1 || limit > 100 {
		limit = 10
	}

	offset := (page - 1) * limit
	log.Printf("[API] Processed params - page: %d, limit: %d, offset: %d", page, limit, offset)

	// 総件数を取得
	log.Println("[DB] Executing count query...")
	var totalCount int
	err = h.db.Get(&totalCount, "SELECT COUNT(*) FROM products")
	if err != nil {
		log.Printf("[DB ERROR] Failed to get total count: %v", err)
		span.SetAttributes(attribute.String("error", err.Error()))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	log.Printf("[DB] Total products count: %d", totalCount)

	// 製品データを取得
	log.Printf("[DB] Executing products query with limit: %d, offset: %d", limit, offset)
	products := []models.Product{}
	query := "SELECT id, name, category, brand, model, description, price, created_at FROM products ORDER BY id LIMIT ? OFFSET ?"
	err = h.db.Select(&products, query, limit, offset)
	if err != nil {
		log.Printf("[DB ERROR] Failed to get products: %v", err)
		span.SetAttributes(attribute.String("error", err.Error()))
		http.Error(w, "Internal server error", http.StatusInternalServerError)
		return
	}
	log.Printf("[DB] Retrieved %d products", len(products))

	totalPages := int(math.Ceil(float64(totalCount) / float64(limit)))
	log.Printf("[API] Calculated total pages: %d", totalPages)

	span.SetAttributes(
		attribute.Int("page", page),
		attribute.Int("limit", limit),
		// ↑上の2行を追加
		attribute.Int("total_count", totalCount),
		attribute.Int("total_pages", totalPages),
		attribute.Int("returned_count", len(products)),
	)

	response := models.PaginatedResponse{
		Products:   products,
		Page:       page,
		Limit:      limit,
		TotalPages: totalPages,
		Count:      totalCount,
	}

	if err := json.NewEncoder(w).Encode(response); err != nil {
		log.Printf("[ERROR] Failed to encode products response: %v", err)
		return
	}

	duration := time.Since(start)
	log.Printf("[API] Get products completed in %v - returned %d products", duration, len(products))
}
