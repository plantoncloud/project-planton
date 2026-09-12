<#ftl output_format="plainText">
<#import "template.ftl" as layout>
<@layout.textLayout>
An administrator of ${properties.brandName} asked you to update your account: <#if requiredActions??><#list requiredActions as reqActionItem>${msg("requiredAction.${reqActionItem}")}<#sep>, </#sep></#list><#else>${msg("executeActionsGeneric")}</#if>. Start here:
${link}

This link works for ${linkExpirationFormatter(linkExpiration)}.

If you were not expecting this, you can ignore this email; nothing changes until you act.
</@layout.textLayout>
