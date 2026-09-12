<#-- The identity server's own "Test connection" from its admin console:
     proves the realm's relay in the same frame as every other email. -->
<#import "template.ftl" as layout>
<@layout.emailLayout preview=msg("emailTestPreview")>
<@layout.heading>Your identity server can send email</@layout.heading>
<@layout.paragraph>This message was sent by the identity server behind ${properties.brandName} through the relay it was given. If it reached you, password resets reach your teammates the same way.</@layout.paragraph>
<@layout.muted>Sent on request from the identity server's admin console. Nobody else received it.</@layout.muted>
</@layout.emailLayout>
