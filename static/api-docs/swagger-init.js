(function () {
    "use strict";

    var element = document.getElementById("swagger-ui");
    if (!element || typeof SwaggerUIBundle !== "function") {
        return;
    }

    window.ui = SwaggerUIBundle({
        url: element.getAttribute("data-openapi-url") || "./openapi.yaml",
        dom_id: "#swagger-ui",
        deepLinking: true,
        docExpansion: "list",
        defaultModelsExpandDepth: 1,
        validatorUrl: null,
        supportedSubmitMethods: [],
        presets: [
            SwaggerUIBundle.presets.apis,
        ],
        layout: "BaseLayout",
    });
}());
