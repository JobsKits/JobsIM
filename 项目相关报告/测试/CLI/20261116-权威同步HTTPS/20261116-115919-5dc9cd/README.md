# CLI schema1 首轮观察保留

本轮10项观察退出0，原Driver及Assertions/result.json不改。随后自查发现：坏媒体尚未把具体HTTP状态／错误码写断言，退群后云端精确GET也未独立断言；其 actualRejection 文字属于夹具预期，不能单凭该轮证明明确拒绝。原执行源已另存 source-before-refinement.py 与 SHA。

[最终加强轮](../20261116-120401-3cbd4e/README.md)使用全新账号与目录，加入实际403 OBJECT_NOT_OWNED／退群精确GET403 NOT_A_MEMBER断言，10项重新全部执行通过。两轮不累加为20项，不用新源码追认首轮。
