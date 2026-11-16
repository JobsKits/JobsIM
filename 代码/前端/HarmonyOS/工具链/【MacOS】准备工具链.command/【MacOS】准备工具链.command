#!/bin/zsh
# shell: zsh
# Created by Jobs on 2026年11月15日，星期日.
initialize() {
    set -eu
    setopt PIPE_FAIL NO_NOMATCH
    umask 077
    CACHE="${JOBS_IM_HARMONY_CACHE:-$HOME/Library/Application Support/JobsIM/Toolchains/HarmonyMirror}"
    CLT="${JOBS_IM_HARMONY_CLT_HOME:-$CACHE/ArkTS-6.1.1.280/command-line-tools}"
    SOURCE='https://libsdl.org/harmonyos-devtools/commandline-tools-mac-arm64-6.1.1.280.zip'
    local script_dir="${${(%):-%x}:A:h}" project_root
    project_root="${script_dir}/../../../../.."; project_root="${project_root:A}"
    [[ "$CACHE" == /* && "$CACHE" != "$project_root" && "$CACHE" != "$project_root"/* && ! -L "$CACHE" ]] || { print -u2 '公共工具缓存必须位于源码外独立目录。'; return 2; }
    [[ "$CLT" == /* && "$CLT" != "$project_root" && "$CLT" != "$project_root"/* && ! -L "$CLT" ]] || { print -u2 'CLT须为源码外独立绝对目录。'; return 2; }
}
check_tools() {
    [[ "$(uname -s)" == Darwin && "$(uname -m)" == arm64 ]] || { print -u2 '此准备入口需要MacOS arm64。'; return 2; }
    [[ -x "$CLT/tool/node/bin/node" && -f "$CLT/hvigor/hvigor/bin/hvigor.js" && -f "$CLT/sdk/default/sdk-pkg.json" ]] || { print -u2 '缺镜像CLT；显式prepare下载，或指定已准备的JOBS_IM_HARMONY_CLT_HOME。'; return 2; }
    file "$CLT/tool/node/bin/node" | /usr/bin/grep -q 'arm64' || { print -u2 'Node不匹配arm64，拒绝执行。'; return 2; }
    "$CLT/tool/node/bin/node" - "$CLT" <<'JS'
const fs=require('fs'),path=require('path'),root=process.argv[2];
const read=p=>JSON.parse(fs.readFileSync(path.join(root,p),'utf8'));
const hvigor=read('hvigor/hvigor/package.json');
const sdk=read('sdk/default/sdk-pkg.json').data;
if(hvigor.version!=='6.24.2'||sdk.apiVersion!=='24'||sdk.version!=='6.1.1.125')throw Error('Mirror toolchain does not match the project lock');
for(const distribution of ['openharmony','hms']){
    for(const component of ['ets','js','native','previewer','toolchains']){
        const base=path.join(root,'sdk/default',distribution,component);
        const manifest=['oh-uni-package.json','uni-package.json'].map(name=>path.join(base,name)).find(file=>fs.existsSync(file));
        if(!manifest)throw Error('Missing SDK component metadata: '+distribution+'/'+component);
        JSON.parse(fs.readFileSync(manifest,'utf8'));
    }
}
const libraryRoots=['sdk/default/hms/toolchains/lib','sdk/default/openharmony/previewer/common/bin'];
for(const name of ['libimage_transcoder_shared.dylib','libsec_shared.dylib','libcjson.dylib','libhilog.dylib','libskia_canvaskit.dylib','libjsoncpp.dylib']){
    if(!libraryRoots.some(base=>fs.existsSync(path.join(root,base,name))))throw Error('Missing resource compiler library: '+name);
}
console.log('Node '+process.version+'; Hvigor '+hvigor.version+'; SDK '+sdk.version+' / API'+sdk.apiVersion);
JS
    print "CLT=$CLT"
    if [[ -f "$CACHE/selected-extraction.json" && "$CLT" == "$CACHE/ArkTS-6.1.1.280/command-line-tools" ]]; then
        print '缓存来源：libsdl.org/harmonyos-devtools/；部分获取的逐文件CRC/SHA在selected-extraction及extra清单。'
    else
        print '复用显式工具缓存，保留其实际来源；新下载仅从libsdl.org。'
    fi
    print '构建器、组件元数据及本工程已知资源编译依赖齐备；实际HAP、签名、模拟器及真机分别验收。'
}
prepare_tools() {
    if check_tools 2>/dev/null; then print '复用健康镜像缓存，不重复下载或删除既有官方缓存。'; return 0; fi
    print "来源：$SOURCE"
    print "目标：$CACHE"
    print '将下载完整CLT约1.9GiB，ZIP逐项CRC检查并记录本地SHA256；该SHA256不冒充发布方签名或官方摘要。'
    print '仅当前下载进程使用显式代理参数；不改系统代理、全局SDK、CA或Shell启动文件。Ctrl+C取消，保留可续传part。'
    [[ ! -e "$CLT" ]] || { print -u2 '既有CLT不完整或版本不符，保留原件；请用新的独立缓存路径，不覆盖未知内容。'; return 2; }
    mkdir -p "$CACHE/Downloads"
    local archive="$CACHE/Downloads/commandline-tools-mac-arm64-6.1.1.280.full.zip"
    local -a proxy_args; proxy_args=()
    [[ -z "${JOBS_IM_DOWNLOAD_PROXY:-}" ]] || proxy_args=(--proxy "$JOBS_IM_DOWNLOAD_PROXY")
    curl --fail --location --show-error --proto '=https' --proto-redir '=https' --retry 3 --continue-at - "${proxy_args[@]}" --output "$archive.part" "$SOURCE"
    unzip -tq "$archive.part" || { print -u2 '完整ZIP校验失败，未执行或解压为工具。'; return 2; }
    mv -- "$archive.part" "$archive"
    shasum -a 256 "$archive" > "$archive.sha256"
    mkdir -p "${CLT:h}"
    local extraction="$(mktemp -d "$CACHE/FullExtract.XXXXXX")"
    unzip -q "$archive" -d "$extraction"
    [[ -d "$extraction/command-line-tools" && ! -e "$CLT" ]] || { print -u2 '归档布局或目标发生变化，保留原件并停止。'; return 2; }
    mv -- "$extraction/command-line-tools" "$CLT"
    print -r -- "$SOURCE" > "$archive.source"
    check_tools
}
main() {
    initialize
    case "${1:-check}" in
        check) check_tools ;;
        prepare) prepare_tools ;;
        *) print -u2 '用法：准备工具链.command check|prepare'; return 2 ;;
    esac
}
main "$@"
