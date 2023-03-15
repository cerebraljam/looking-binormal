package main

import (
	"fmt"
	"net/http"
	"os"

	"context"

	"github.com/gin-gonic/gin"
)

var (
	redisURL        = os.Getenv("REDIS_URL")
	redisSecretName = os.Getenv("REDIS_SECRET_NAME")
	environment     = os.Getenv("ENV")
	listenAddr      = os.Getenv("LISTEN_ADDRESS")
)

func main() {
	fmt.Println("Starting looking binormal ...")

	ctx := context.TODO()
	database, err := newDatabase(ctx, redisURL, redisSecretName)
	if err != nil {
		fmt.Println("FATAL ERROR: couldn't connect to redis:", err)
	}

	if listenAddr == "" {
		listenAddr = "127.0.0.1:8080"
	}

	router := initRouter(database)
	router.Run(listenAddr)

	fmt.Println("Stopped looking binormal ...")
}

func initRouter(database *database) *gin.Engine {
	r := gin.Default()
	hub := newHub()
	go hub.run()

	r.LoadHTMLGlob("static/*.html")

	r.SetTrustedProxies(nil)

	if environment != "dev" {
		gin.SetMode(gin.ReleaseMode)
	}

	r.GET("/ping", pingHandler)

	r.GET("/", func(c *gin.Context) {
		c.HTML(http.StatusOK, "index.html", nil)
	})

	r.POST("/discrete", func(c *gin.Context) {
		discreteHandler(c, database, hub)
	})

	r.GET("/websocket", func(c *gin.Context) {
		serveWs(hub, c.Writer, c.Request)
	})

	return r
}
