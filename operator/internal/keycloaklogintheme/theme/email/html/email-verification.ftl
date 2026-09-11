<#-- The person's email address is being verified. -->
<#import "template.ftl" as layout>
<@layout.emailLayout preview=msg("emailVerificationPreview")>
<@layout.heading>Verify your email address</@layout.heading>
<@layout.paragraph>A ${properties.brandName} account was created with this address. If that was you, confirm it below.</@layout.paragraph>
<@layout.button href=link>Verify Email Address</@layout.button>
<@layout.fallbackLink href=link/>
<@layout.muted>This link works for ${linkExpirationFormatter(linkExpiration)}.</@layout.muted>
<@layout.muted>If you did not create this account, you can ignore this email.</@layout.muted>
</@layout.emailLayout>
