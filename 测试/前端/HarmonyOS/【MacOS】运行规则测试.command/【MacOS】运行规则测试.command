#!/bin/zsh
# shell: zsh
# Created by Jobs on 2026年11月15日，星期日.
set -eu
setopt PIPE_FAIL NO_NOMATCH
SCRIPT_DIR="${${(%):-%x}:A:h}"
REPO="${SCRIPT_DIR}/../../../.."; REPO="${REPO:A}"
CACHE="${JOBS_IM_HARMONY_CACHE:-$HOME/Library/Application Support/JobsIM/Toolchains/HarmonyMirror}"
[[ "$CACHE" == /* && "$CACHE" != "$REPO" && "$CACHE" != "$REPO"/* ]] || { print -u2 '测试缓存必须位于源码外绝对目录。'; exit 2; }
CLT="${JOBS_IM_HARMONY_CLT_HOME:-$CACHE/ArkTS-6.1.1.280/command-line-tools}"
NODE="$CLT/tool/node/bin/node"
TYPESCRIPT="$CLT/sdk/default/openharmony/ets/build-tools/ets-loader/node_modules/typescript"
[[ -x "$NODE" && -f "$TYPESCRIPT/package.json" ]] || { print -u2 '缺镜像SDK；不会在线安装其它源。'; exit 2; }
WORK="$(mktemp -d "$CACHE/HostRules.XXXXXX")"
REPORT="${JOBS_IM_HARMONY_REPORT_DIR:-$REPO/项目相关报告/测试/前端/HarmonyOS/$(date +%Y%m%d-%H%M%S)-主机规则}"
REPORT="${REPORT:A}"
mkdir -p "$REPORT"
"$NODE" - "$TYPESCRIPT" "$REPO/代码/前端/HarmonyOS/entry/src/main/ets/Data" "$WORK" <<'JS'
const fs=require('fs'),path=require('path');
const ts=require(process.argv[2]);
for(const name of ['Models','Strings']){
 const source=fs.readFileSync(path.join(process.argv[3],name+'.ets'),'utf8');
 const result=ts.transpileModule(source,{compilerOptions:{target:ts.ScriptTarget.ES2020,module:ts.ModuleKind.CommonJS},fileName:name+'.ts'});
 fs.writeFileSync(path.join(process.argv[4],name+'.cjs'),result.outputText);
}
JS
print '运行当前自有纯规则源码的主机回归；不代表ArkUI、RDB、AssetStore或设备Hypium已执行。'
JOBS_IM_HARMONY_UI_SOURCE="$REPO/代码/前端/HarmonyOS/entry/src/main/ets/pages/Index.ets" JOBS_IM_HARMONY_HOST_MODELS="$WORK/Models.cjs" JOBS_IM_HARMONY_HOST_STRINGS="$WORK/Strings.cjs" "$NODE" --test "$REPO/测试/前端/HarmonyOS/Unit/Policies.test.mjs" 2>&1 | tee "$REPORT/规则结果.txt"
