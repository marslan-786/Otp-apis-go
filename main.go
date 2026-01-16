package main

import (
	"log"
	"net/http"
	"os"

	"myproject/dgroup"
	"myproject/mait"
	"myproject/npmneon"
	

	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()

	// =================================================================
	// اہم تبدیلی: NewClient کی جگہ اب GetSession کال ہوگا
	// یہ فنکشن اب RAM سے سیشن اٹھائے گا اور ہارڈ کوڈڈ لاگ ان استعمال کرے گا
	// =================================================================
	
	dClient := dgroup.GetSession()
	
	neonClient := npmneon.GetSession()
	maitClient := mait.GetSession()

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
	
	// ================= NPM-NEON ROUTES =================
	r.GET("/npm-neon/sms", func(c *gin.Context) {
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

	// ================= MAIT (Masdar) ROUTES =================
	r.GET("/maits/sms", func(c *gin.Context) {
		data, err := maitClient.GetSMSLogs()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Data(http.StatusOK, "application/json", data)
	})

	r.GET("/maits/numbers", func(c *gin.Context) {
		data, err := maitClient.GetNumberStats()
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
