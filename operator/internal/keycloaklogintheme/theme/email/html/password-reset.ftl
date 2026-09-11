<#-- A teammate asked to reset their password from the sign-in page. Composed
     from the layout macros so it is the same design as every email the
     platform sends; Keycloak supplies link, linkExpiration, and the
     linkExpirationFormatter. -->
<#import "template.ftl" as layout>
<@layout.emailLayout preview=msg("passwordResetPreview")>
<@layout.heading>Reset your password</@layout.heading>
<@layout.paragraph>Someone asked to reset the password for your ${properties.brandName} account. If that was you, choose a new password below.</@layout.paragraph>
<@layout.button href=link>Choose a New Password</@layout.button>
<#-- The action token behind the button runs to a thousand characters: a
     labelled link keeps a way in when a client strips the button, without
     a wall of token no one can check by eye. The text twin carries the URL. -->
<@layout.fallbackAction href=link>the password reset link</@layout.fallbackAction>
<@layout.muted>This link works for ${linkExpirationFormatter(linkExpiration)}.</@layout.muted>
<@layout.muted>If you did not ask for this, you can ignore this email; your password stays as it is.</@layout.muted>
</@layout.emailLayout>
