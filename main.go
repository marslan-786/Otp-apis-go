package main

import (
	"log"
	"net/http"
	
	// Import ese karna hai: "ProjectName / FolderName"
	"myproject/dgroup"

	"github.com/gin-gonic/gin"
)

func main() {
	r := gin.Default()

	// Initialize D-Group Logic (Background Memory starts here)
	dClient := dgroup.NewClient()

	// ------------------ D-GROUP ROUTES ------------------
	r.GET("/d-group/sms", func(c *gin.Context) {
		// Ye function automatically alag thread (Goroutine) me chalta hai Gin k andar
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

	// Future: Yahan hum mazeed panels add karengy
	// himalayaClient := himalaya.NewClient()
	// r.GET("/himalaya/sms", ...)

	log.Println("Server running on port 8080")
	r.Run(":8080")
}
