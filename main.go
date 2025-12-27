package main

import (
	"log"
	"net/http"
	"os"

	// Dono packages import karein
	"myproject/dgroup"
	"myproject/numberpanel" // <--- NEW IMPORT
	"myproject/npmneon"

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
	
	
	r.GET("/npm-neon/sms", func(c *gin.Context) {
		data, err := neonClient.GetSMSLogs()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		// Ab data clean ho kar JSON format me hi jayega
		c.Data(http.StatusOK, "application/json", data)
	})

	r.GET("/npm-neon/numbers", func(c *gin.Context) {
		data, err := neonClient.GetNumberStats()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Data(http.StatusOK, "application/json", data)
	})

	// Server Start Logic
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Println("Server running on port: " + port)
	r.Run("0.0.0.0:" + port)
}