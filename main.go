package main

import (
	"log"
	"net/http"
	"os" // <--- Ye Import lazmi add krna

	"myproject/dgroup"

	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()

	dClient := dgroup.NewClient()

	r.GET("/d-group/sms", func(c *gin.Context) {
		data, err := dClient.GetSMSLogs()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Data(http.StatusOK, "application/json", data)
	})

	r.GET("/d-group/numbers", func(c *gin.Context) {
		data, err := dClient.GetNumberStats()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Data(http.StatusOK, "application/json", data)
	})

	// ---------------------------------------------------------
	// PORT FIX FOR RAILWAY
	// Railway automatically injects a PORT env var.
	// We must listen on THAT port, not just 8080.
	// ---------------------------------------------------------
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080" // Localhost k liye fallback
	}

	log.Println("Server running on port: " + port)
	// 0.0.0.0 lagana zaroori hai ta k external traffic accept kare
	r.Run("0.0.0.0:" + port) 
}
