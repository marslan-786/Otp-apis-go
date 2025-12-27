package main

import (
	"log"
	"net/http"
	"os"

	"myproject/dgroup"
	"myproject/numberpanel"
	"myproject/npmneon" // Import मौजूद hai

	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()

	// ---------------- INITIALIZATION (Ye Teeno Lines Zaroori Hain) ----------------
	dClient := dgroup.NewClient()
	npClient := numberpanel.NewClient()
	
	// *** YE WALI LINE MISSING THI ***
	neonClient := npmneon.NewClient() 
	// ********************************

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

	// ================= NUMBER PANEL ROUTES =================
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

	// ================= NPM-NEON ROUTES =================
	r.GET("/npm-neon/sms", func(c *gin.Context) {
		// Ab neonClient defined hai, error nahi ayega
		data, err := neonClient.GetSMSLogs()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
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

	// ================= SERVER START =================
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	log.Println("Server running on port: " + port)
	r.Run("0.0.0.0:" + port)
}
