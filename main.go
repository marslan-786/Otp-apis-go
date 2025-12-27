package main

import (
	"log"
	"net/http"
	"os"

	// Dono packages import karein
	"myproject/dgroup"
	"myproject/numberpanel" // <--- NEW IMPORT

	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()

	// 1. Initialize D-Group
	dClient := dgroup.NewClient()

	// 2. Initialize Number Panel (NEW)
	npClient := numberpanel.NewClient()

	// ================= D-GROUP ROUTES =================
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

	// ================= NUMBER PANEL ROUTES (NEW) =================
	r.GET("/number-panel/sms", func(c *gin.Context) {
		data, err := npClient.GetSMSLogs()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Data(http.StatusOK, "application/json", data)
	})

	r.GET("/number-panel/numbers", func(c *gin.Context) {
		data, err := npClient.GetNumberStats()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Data(http.StatusOK, "application/json", data)
	})

	// ================= SERVER START =================
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Println("Server running on port: " + port)
	r.Run("0.0.0.0:" + port)
}
