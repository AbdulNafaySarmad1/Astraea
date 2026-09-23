# Nocturn Keycloak login theme

The `themes/nocturn/login` directory extends Keycloak's `keycloak.v2` login theme. It adds the Nocturn mark to the inherited realm header and registers the same image as the login-page favicon. It does not replace authentication templates or change Keycloak's forms.

Deploy this directory as a versioned theme to the Keycloak server's `themes/nocturn` path, or package it as a theme archive for clustered production deployment. In **Realm settings → Themes**, select `nocturn` for the login theme. Set the realm display name to the approved product name. Verify login, MFA, recovery, error, and logout pages in staging after every Keycloak upgrade; the theme is prepared here but no Keycloak server is included in the local demo.

Theme creation, stylesheet inheritance, favicon properties, and deployment follow the [Keycloak theme guide](https://www.keycloak.org/ui-customization/themes). The parent theme choice follows Keycloak's [v2 login-theme release note](https://www.keycloak.org/docs/latest/release_notes/).
