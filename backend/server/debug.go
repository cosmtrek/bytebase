package server

import (
	"fmt"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

func (s *Server) registerDebugRoutes(e *echo.Echo) {
	e.GET("/api/ping", func(c echo.Context) error {
		return c.String(http.StatusOK, fmt.Sprintf("[%v] Pong from backend", time.Now().Format("2006-01-02 15:04:05")))
	})

}