package router

import (
	"github.com/faisalaffan/ewallet-system/docs"
	"github.com/faisalaffan/ewallet-system/internal/handler"
	"github.com/faisalaffan/ewallet-system/internal/middleware"
	"github.com/faisalaffan/ewallet-system/pkg/telemetry"
	"github.com/gofiber/fiber/v3"
)

const swaggerHTML = `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="utf-8" />
    <meta name="viewport" content="width=device-width, initial-scale=1" />
    <title>E-Wallet API Docs</title>
    <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist@5/swagger-ui.css" />
</head>
<body>
    <div id="swagger-ui"></div>
    <script src="https://unpkg.com/swagger-ui-dist@5/swagger-ui-bundle.js" crossorigin></script>
    <script>
        SwaggerUIBundle({ url: "/swagger/doc.json", dom_id: "#swagger-ui" });
    </script>
</body>
</html>`

func Setup(h *handler.WalletHandler) *fiber.App {
	app := fiber.New()

	app.Use(middleware.OtelMiddleware)
	app.Use(telemetry.MetricsMiddleware)

	app.Get("/swagger", func(c fiber.Ctx) error {
		c.Set("Content-Type", "text/html")
		return c.SendString(swaggerHTML)
	})
	app.Get("/swagger/doc.json", func(c fiber.Ctx) error {
		c.Set("Content-Type", "application/json")
		return c.SendString(docs.SwaggerInfo.ReadDoc())
	})

	api := app.Group("/api")
	api.Use(middleware.AuthMiddleware)
	api.Post("/wallets", h.Create)
	api.Get("/wallets/:id", h.Get)
	api.Post("/wallets/:id/topup", h.TopUp)
	api.Post("/wallets/:id/pay", h.Pay)
	api.Post("/wallets/transfer", h.Transfer)
	api.Post("/wallets/:id/suspend", h.Suspend)
	api.Get("/wallets/:id/reconcile", h.Reconcile)

	return app
}
