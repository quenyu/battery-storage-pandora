window.onload = function () {
  window.ui = SwaggerUIBundle({
    url: "/openapi.yaml",
    dom_id: "#swagger-ui",
    deepLinking: true,
    displayRequestDuration: true,
    validatorUrl: null,
    queryConfigEnabled: false,
    presets: [SwaggerUIBundle.presets.apis],
    layout: "BaseLayout"
  });
};
