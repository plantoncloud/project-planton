<#ftl output_format="plainText">
<#import "template.ftl" as layout>
<@layout.textLayout>
A ${properties.brandName} account was created with this address. If that was you, confirm it here:
${link}

This link works for ${linkExpirationFormatter(linkExpiration)}.

If you did not create this account, you can ignore this email.
</@layout.textLayout>
