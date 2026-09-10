package keycloaklogintheme

// emailThemeProperties is the manifest of the theme's EMAIL type -- the
// templates behind every email the identity server sends (password reset,
// verify email, an administrator's required-action mail).
//
// It exists because the server-wide default-theme flag on the identity
// Deployment (--spi-theme--default=planton) names this theme for EVERY theme
// type, not only login. Keycloak resolves the email type of the default
// theme when it renders a message; a theme directory with no email type is
// "Failed to find EMAIL theme planton" and a null renderer -- no password
// reset can be sent at all. So the theme declares an email type that
// inherits Keycloak's bundled email templates whole (parent=keycloak): the
// messages render in the server's stock look until the Planton-branded email
// templates land here, and that landing is a change to THIS type only.
const emailThemeProperties = `parent=keycloak
import=common/keycloak
`
