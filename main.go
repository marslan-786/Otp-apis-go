package main

import (
	"log"
	"net/http"
	"os"

	"myproject/dgroup"
	"myproject/numberpanel"
	"myproject/npmneon"
	"myproject/mait" // <--- NEW IMPORT

	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()

	// Initializations
	dClient := dgroup.NewClient()
	npClient := numberpanel.NewClient()
	neonClient := npmneon.NewClient()
	maitClient := mait.NewClient() // <--- INIT

	// ... (Previous Routes D-Group, NumberPanel, Neon) ...
	// (Mai purane routes dobara likh k lambi nahi kar raha, wo wese hi rahengy)
	
	// ----- SIRF YE WALE ADD KARNE HAIN (Previous routes k neechay) -----

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

	// ================= mait (MAIT) ROUTES (NEW) =================
	r.GET("/mait/sms", func(c *gin.Context) {
		data, err := maitClient.GetSMSLogs()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		c.Data(http.StatusOK, "application/json", data)
	})

	r.GET("/mait/numbers", func(c *gin.Context) {
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
