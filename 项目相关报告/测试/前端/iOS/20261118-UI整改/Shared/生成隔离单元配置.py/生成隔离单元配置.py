#!/usr/bin/env python3
"""只生成当前产品的本地单元测试配置；不启动测试或业务服务。"""

import argparse
import copy
import hashlib
import json
import plistlib
import sys
from pathlib import Path


LOCAL_CLASSES = (
    "IMAuthCredentialTests", "IMAuthoritativeSyncTests", "IMDataExportTests",
    "IMMigrationServiceTests", "IMMigrationTests", "IMPersonalStateTests",
    "IMReceiptServiceTests", "IMReceiptStorageTests", "IMSessionTests",
    "IMStorageTests", "IMRememberedLoginTests",
)


# 只替换产品根占位，保留由Xcode解析的系统和host占位。
def relocate(value, products):
    if isinstance(value, str):
        return value.replace("__TESTROOT__", str(products))
    if isinstance(value, list):
        return [relocate(item, products) for item in value]
    if isinstance(value, dict):
        return {key: relocate(item, products) for key, item in value.items()}
    return value


# 验证当前编译产物并生成唯一白名单配置，既有输出不覆盖。
def generate(options):
    template = options.template.expanduser().resolve(strict=True)
    products = options.products.expanduser().resolve(strict=True)
    output = options.output.expanduser().absolute()
    metadata_path = output.with_suffix(".json")
    if (output.exists() or output.is_symlink() or metadata_path.exists() or metadata_path.is_symlink()
            or output.suffix != ".xctestrun"):
        raise ValueError("输出必须是尚不存在的.xctestrun文件")
    host = products / "Debug-iphonesimulator/JobsIM.app"
    bundle = host / "PlugIns/JobsIMTests.xctest"
    host_info = plistlib.loads((host / "Info.plist").read_bytes())
    if host_info.get("CFBundleIdentifier") != "com.jobs.im":
        raise ValueError("不是当前JobsIM应用")
    if host_info.get("DTPlatformName") != "iphonesimulator":
        raise ValueError("只接受已构建的iOS模拟器产品")
    executable = host / host_info["CFBundleExecutable"]
    test_info = plistlib.loads((bundle / "Info.plist").read_bytes())
    test_executable = bundle / test_info["CFBundleExecutable"]
    test_bytes = test_executable.read_bytes()
    if any(name.encode() not in test_bytes for name in LOCAL_CLASSES):
        raise ValueError("当前测试二进制未包含完整本地白名单，请先完成build-for-testing")
    source = plistlib.loads(template.read_bytes())
    if source.get("__xctestrun_metadata__", {}).get("FormatVersion") != 1:
        raise ValueError("只支持已审核的FormatVersion=1模板")
    entry = copy.deepcopy(source["JobsIMTests"])
    if entry.get("IsAppHostedTestBundle") is not True or entry.get("ProductModuleName") != "JobsIMTests":
        raise ValueError("模板不是JobsIM app-hosted单元测试")
    if not entry.get("TestHostPath", "").startswith("__TESTROOT__/"):
        raise ValueError("模板host必须使用产品根占位，拒绝沿用旧绝对产品")
    entry = relocate(entry, products)
    entry["TestHostPath"] = str(host)
    entry["TestBundlePath"] = "__TESTHOST__/PlugIns/JobsIMTests.xctest"
    entry["DependentProductPaths"] = [str(host), str(bundle)]
    entry["CommandLineArguments"] = []
    entry["OnlyTestIdentifiers"] = list(LOCAL_CLASSES)
    entry.pop("SkipTestIdentifiers", None)
    for key in ("EnvironmentVariables", "TestingEnvironmentVariables"):
        environment = entry.setdefault(key, {})
        for name in list(environment):
            if name.startswith("JOBS_IM_"):
                del environment[name]
        environment["JOBS_IM_SYNC_INJECTED_TESTS"] = "1"
    result = {"JobsIMTests": entry, "__xctestrun_metadata__": source["__xctestrun_metadata__"]}
    output.parent.mkdir(parents=True, exist_ok=True)
    with output.open("xb") as stream:
        plistlib.dump(result, stream)
    metadata = {
        "configuration": str(output), "template": str(template), "products": str(products),
        "classes": list(LOCAL_CLASSES), "apiUsed": False, "testExecuted": False,
        "injection": "JOBS_IM_SYNC_INJECTED_TESTS=1 in both host environment dictionaries",
        "hostSHA256": hashlib.sha256(executable.read_bytes()).hexdigest(),
        "testSHA256": hashlib.sha256(test_bytes).hexdigest(),
    }
    with metadata_path.open("x", encoding="utf-8") as stream:
        json.dump(metadata, stream, ensure_ascii=False, indent=2)
        stream.write("\n")
    print("已生成本地单元白名单配置（未执行测试）：", output)
    print("CLI再附加以下选择器：")
    for name in LOCAL_CLASSES:
        print("-only-testing:JobsIMTests/" + name)


# 参数全部显式指定；调用工作目录和旧模板所在目录均不作为产品根。
def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--template", type=Path, required=True)
    parser.add_argument("--products", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    options = parser.parse_args()
    try:
        generate(options)
    except (OSError, ValueError, KeyError, plistlib.InvalidFileException) as error:
        print("生成失败：" + str(error), file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
