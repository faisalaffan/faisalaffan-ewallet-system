package middleware

import "github.com/gofiber/fiber/v3"

func CORSMiddleware(c fiber.Ctx) error {
	c.Set("Access-Control-Allow-Origin", "*")
	c.Set("Access-Control-Allow-Methods", "GET,POST,PUT,DELETE,OPTIONS")
	c.Set("Access-Control-Allow-Headers", "Authorization,Content-Type")

	if c.Method() == "OPTIONS" {
		return c.SendStatus(204)
	}
	return c.Next()
}
