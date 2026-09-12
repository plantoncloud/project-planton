<#ftl output_format="plainText">
<#import "template.ftl" as layout>
<@layout.textLayout>
Someone asked to reset the password for your ${properties.brandName} account. If that was you, choose a new password here:
${link}

This link works for ${linkExpirationFormatter(linkExpiration)}.

If you did not ask for this, you can ignore this email; your password stays as it is.
</@layout.textLayout>
