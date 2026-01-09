package main

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	_ "github.com/go-sql-driver/mysql"
)

// Database configuration
const (
	dbUser     = "MGAide"
	dbPassword = "2550505fox"
	dbHost     = "1Panel-mysql-dgO0"
	dbPort     = "3306"
	dbName     = "mgaide"
)

var db *sql.DB

// Models
type UserSyncRequest struct {
	UID      string `json:"uid" binding:"required"`
	Username string `json:"username"`
}

type SyncPullResponse struct {
	Favorites              interface{} `json:"favorites"`
	FmFavorites            interface{} `json:"fmFavorites"`
	Playlists              interface{} `json:"playlists"`
	FavoritesUpdatedAt     int64       `json:"favoritesUpdatedAt"`
	FmFavoritesUpdatedAt   int64       `json:"fmFavoritesUpdatedAt"`
	PlaylistsUpdatedAt     int64       `json:"playlistsUpdatedAt"`
}

type SyncPushRequest struct {
	UID                  string      `json:"uid" binding:"required"`
	Favorites            interface{} `json:"favorites"`
	FmFavorites          interface{} `json:"fmFavorites"`
	Playlists            interface{} `json:"playlists"`
	FavoritesUpdatedAt   int64       `json:"favoritesUpdatedAt"`
	FmFavoritesUpdatedAt int64       `json:"fmFavoritesUpdatedAt"`
	PlaylistsUpdatedAt   int64       `json:"playlistsUpdatedAt"`
}

func initDB() {
	dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?charset=utf8mb4&parseTime=True&loc=Local",
		dbUser, dbPassword, dbHost, dbPort, dbName)
	
	var err error
	db, err = sql.Open("mysql", dsn)
	if err != nil {
		log.Fatalf("Error opening database: %v", err)
	}

	// Connection pool settings
	db.SetMaxOpenConns(10)
	db.SetMaxIdleConns(5)
	db.SetConnMaxLifetime(time.Minute * 5)

	if err = db.Ping(); err != nil {
		log.Fatalf("Error connecting to the database: %v", err)
	}
	log.Println("Database connection established")
}

func main() {
	initDB()
	defer db.Close()

	r := gin.Default()

	// CORS Middleware
	r.Use(func(c *gin.Context) {
		c.Writer.Header().Set("Access-Control-Allow-Origin", "*")
		c.Writer.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		c.Writer.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(204)
			return
		}
		c.Next()
	})

	// Routes group to handle both / and /mgaide-api prefixes
	api := r.Group("/")
	mgaideApi := r.Group("/mgaide-api")

	handlers := func(rg *gin.RouterGroup) {
		// 1. User Login/Sync
		rg.POST("/user/sync", func(c *gin.Context) {
			var req UserSyncRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "uid_required"})
				return
			}

			query := `INSERT INTO users (uid, username, last_seen_at) 
					  VALUES (?, ?, NOW()) 
					  ON DUPLICATE KEY UPDATE username = VALUES(username), last_seen_at = NOW()`
			
			_, err := db.Exec(query, req.UID, req.Username)
			if err != nil {
				log.Printf("DB Error (user/sync): %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"ok": true})
		})

		// 2. Pull Data
		rg.GET("/sync/pull", func(c *gin.Context) {
			uid := c.Query("uid")
			if uid == "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": "uid_required"})
				return
			}

			query := "SELECT favorites_json, fm_favorites_json, playlists_json, favorites_updated_at, fm_favorites_updated_at, playlists_updated_at FROM sync_data WHERE uid = ?"
			var favJSON, fmJSON, playJSON []byte
			var favUp, fmUp, playUp int64

			err := db.QueryRow(query, uid).Scan(&favJSON, &fmJSON, &playJSON, &favUp, &fmUp, &playUp)
			
			if err == sql.ErrNoRows {
				c.JSON(http.StatusOK, SyncPullResponse{
					Favorites: []string{}, FmFavorites: []string{}, Playlists: []string{},
				})
				return
			} else if err != nil {
				log.Printf("DB Error (sync/pull): %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
				return
			}

			// Helper to unmarshal JSON or return empty array
			unmarshal := func(data []byte) interface{} {
				var v interface{}
				if len(data) == 0 {
					return []string{}
				}
				json.Unmarshal(data, &v)
				return v
			}

			c.JSON(http.StatusOK, SyncPullResponse{
				Favorites:              unmarshal(favJSON),
				FmFavorites:            unmarshal(fmJSON),
				Playlists:              unmarshal(playJSON),
				FavoritesUpdatedAt:     favUp,
				FmFavoritesUpdatedAt:   fmUp,
				PlaylistsUpdatedAt:     playUp,
			})
		})

		// 3. Push Data
		rg.POST("/sync/push", func(c *gin.Context) {
			var req SyncPushRequest
			if err := c.ShouldBindJSON(&req); err != nil {
				c.JSON(http.StatusBadRequest, gin.H{"error": "uid_required"})
				return
			}

			favJSON, _ := json.Marshal(req.Favorites)
			fmJSON, _ := json.Marshal(req.FmFavorites)
			playJSON, _ := json.Marshal(req.Playlists)

			query := `INSERT INTO sync_data (
						uid, favorites_json, fm_favorites_json, playlists_json, 
						favorites_updated_at, fm_favorites_updated_at, playlists_updated_at
					  ) VALUES (?, ?, ?, ?, ?, ?, ?)
					  ON DUPLICATE KEY UPDATE 
						favorites_json = VALUES(favorites_json),
						fm_favorites_json = VALUES(fm_favorites_json),
						playlists_json = VALUES(playlists_json),
						favorites_updated_at = VALUES(favorites_updated_at),
						fm_favorites_updated_at = VALUES(fm_favorites_updated_at),
						playlists_updated_at = VALUES(playlists_updated_at)`

			_, err := db.Exec(query, req.UID, favJSON, fmJSON, playJSON, 
				req.FavoritesUpdatedAt, req.FmFavoritesUpdatedAt, req.PlaylistsUpdatedAt)
			
			if err != nil {
				log.Printf("DB Error (sync/push): %v", err)
				c.JSON(http.StatusInternalServerError, gin.H{"error": "db_error"})
				return
			}

			c.JSON(http.StatusOK, gin.H{"ok": true})
		})
	}

	handlers(api)
	handlers(mgaideApi)

	log.Println("Server starting on :3000")
	r.Run(":3000")
}
