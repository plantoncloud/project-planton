<#-- An administrator asked the person to complete one or more account
     actions (set a password, verify an email, enroll a second factor).
     Keycloak names the actions in requiredActions; the layout frames it. -->
<#import "template.ftl" as layout>
<@layout.emailLayout preview=msg("executeActionsPreview")>
<@layout.heading>Finish setting up your account</@layout.heading>
<@layout.paragraph>An administrator of ${properties.brandName} asked you to update your account: <#if requiredActions??><#list requiredActions as reqActionItem>${msg("requiredAction.${reqActionItem}")}<#sep>, </#sep></#list><#else>${msg("executeActionsGeneric")}</#if>. Start below.</@layout.paragraph>
<@layout.button href=link>Update Your Account</@layout.button>
<@layout.fallbackLink href=link/>
<@layout.muted>This link works for ${linkExpirationFormatter(linkExpiration)}.</@layout.muted>
<@layout.muted>If you were not expecting this, you can ignore this email; nothing changes until you act.</@layout.muted>
</@layout.emailLayout>
