<#ftl output_format="plainText">
<#import "template.ftl" as layout>
<@layout.textLayout>
This message was sent by the identity server behind ${properties.brandName} through the relay it was given. If it reached you, password resets reach your teammates the same way.

Sent on request from the identity server's admin console. Nobody else received it.
</@layout.textLayout>
