package middleware

import (
	"os"

	"github.com/faisalaffan/ewallet-system/pkg/response"
	"github.com/gofiber/fiber/v3"
)

func AuthMiddleware(c fiber.Ctx) error {
	if c.Method() == "OPTIONS" || c.Path() == "/swagger" || c.Path() == "/swagger/doc.json" {
		return c.Next()
	}

	apiKey := os.Getenv("API_KEY")
	if apiKey == "" {
		return c.Next()
	}

	auth := c.Get("Authorization")
	if len(auth) < 8 || auth[:7] != "Bearer " {
		return c.Status(401).JSON(response.Error(401, "UNAUTHORIZED", "missing or invalid Authorization header"))
	}

	token := auth[7:]
	if token != apiKey {
		return c.Status(401).JSON(response.Error(401, "UNAUTHORIZED", "invalid API key"))
	}

	return c.Next()
}
