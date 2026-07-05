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
        const ui = SwaggerUIBundle({
            url: "/swagger/doc.json",
            dom_id: "#swagger-ui",
            persistAuthorization: true,
            onComplete: function() {
                const stored = localStorage.getItem("ewallet-api-key");
                if (stored) {
                    ui.authActions.authorize({
                        BearerAuth: { name: "BearerAuth", schema: { type: "apiKey", in: "header", name: "Authorization" }, value: stored }
                    });
                }
            }
        });
        document.addEventListener("DOMContentLoaded", function() {
            setTimeout(function() {
                const btn = document.createElement("div");
                btn.innerHTML = '<div class="auth-wrapper" style="margin:10px 0"><input id="api-key-input" type="password" placeholder="API Key" style="padding:6px;width:240px;margin-right:8px;border:1px solid #ccc;border-radius:4px"><button id="api-key-btn" style="padding:6px 12px;cursor:pointer">Set Token</button></div>';
                const target = document.querySelector(".information-container") || document.querySelector(".topbar");
                if (target) target.insertAdjacentHTML("afterend", btn.outerHTML);
                document.getElementById("api-key-btn").addEventListener("click", function() {
                    const key = document.getElementById("api-key-input").value;
                    if (key) {
                        localStorage.setItem("ewallet-api-key", key);
                        ui.authActions.authorize({
                            BearerAuth: { name: "BearerAuth", schema: { type: "apiKey", in: "header", name: "Authorization" }, value: key }
                        });
                        alert("Token set! Klik Authorize lalu coba endpoint.");
                    }
                });
            }, 500);
        });
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
