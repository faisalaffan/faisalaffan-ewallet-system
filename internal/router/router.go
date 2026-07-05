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
        function injectTokenUI() {
            const target = document.querySelector(".information-container") || document.querySelector(".topbar");
            if (!target) return setTimeout(injectTokenUI, 200);
            const wrapper = document.createElement("div");
            wrapper.style.cssText = "margin:10px 0";
            const input = document.createElement("input");
            input.id = "api-key-input";
            input.type = "password";
            input.placeholder = "API Key";
            input.style.cssText = "padding:6px;width:240px;margin-right:8px;border:1px solid #ccc;border-radius:4px";
            const button = document.createElement("button");
            button.id = "api-key-btn";
            button.textContent = "Set Token";
            button.style.cssText = "padding:6px 12px;cursor:pointer";
            button.onclick = function() {
                const key = input.value;
                if (key) {
                    localStorage.setItem("ewallet-api-key", key);
                    ui.authActions.authorize({
                        BearerAuth: { name: "BearerAuth", schema: { type: "apiKey", in: "header", name: "Authorization" }, value: key }
                    });
                    ui.specActions.download();
                }
            };
            wrapper.appendChild(input);
            wrapper.appendChild(button);
            target.parentNode.insertBefore(wrapper, target.nextSibling);
        }
        setTimeout(injectTokenUI, 500);
    </script>
</body>
</html>`

func Setup(h *handler.WalletHandler) *fiber.App {
	app := fiber.New()

	app.Use(middleware.CORSMiddleware)
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
