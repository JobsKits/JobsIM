#!/bin/zsh
# shell: zsh
# Created by Jobs on 2026年11月14日，星期六.
initialize() {
    setopt NO_NOMATCH PIPE_FAIL
    set -eu
    SCRIPT_DIR="${${(%):-%x}:A:h}"
    PROJECT_ROOT="${SCRIPT_DIR}/../../../.."
    PROJECT_ROOT="${PROJECT_ROOT:A}"
    ANDROID_PROJECT="${PROJECT_ROOT}/代码/前端/Android"
    REPORT_DIR="${PROJECT_ROOT}/项目相关报告/测试/前端/Android/$(date +%Y%m%d-%H%M%S)-$$"
    mkdir -p "$REPORT_DIR"
}
choose_target() {
    local mode="$1" line serial
    local -a devices
    devices=()
    while IFS= read -r line; do
        serial="${line%%[[:space:]]*}"
        [[ "$line" == *$'\tdevice'* ]] || continue
        if [[ "$mode" == "emulator" && "$serial" == emulator-* ]] || [[ "$mode" == "device" && "$serial" != emulator-* ]]; then devices+=("$serial"); fi
    done < <("$ADB" devices)
    (( ${#devices} > 0 )) || { print -u2 "无可用${mode}目标；先在 Android Studio启动模拟器，或连接并授权真机。"; return 2; }
    if (( ${#devices} == 1 )); then TARGET="${devices[1]}"
    elif [[ -n "${ANDROID_SERIAL:-}" && "${devices[(Ie)$ANDROID_SERIAL]}" != "0" ]]; then TARGET="$ANDROID_SERIAL"
    elif [[ -t 0 ]] && command -v fzf >/dev/null; then TARGET="$(printf '%s\n' "${devices[@]}" | fzf --prompt='选择Android目标（Esc取消）> ')" || return 130
    else print -u2 '多个目标；交互时安装fzf选择，或显式设置已枚举的ANDROID_SERIAL。'; return 2
    fi
}
run_tests() {
    local mode="$1"
    case "$mode" in
        check) command -v java; java -version; print "Android SDK: ${ANDROID_HOME:-${ANDROID_SDK_ROOT:-未设置}}"; "$ANDROID_PROJECT/gradlew" -p "$ANDROID_PROJECT" --version ;;
        unit|build) "$ANDROID_PROJECT/gradlew" -p "$ANDROID_PROJECT" --no-daemon :app:assembleDebug :app:testDebugUnitTest || return $?
            mkdir -p "$ANDROID_PROJECT/Dist"
            cp "$ANDROID_PROJECT/app/build/outputs/apk/debug/app-debug.apk" "$ANDROID_PROJECT/Dist/JobsIM-debug.apk"
            cp -R "$ANDROID_PROJECT/app/build/test-results/testDebugUnitTest" "$REPORT_DIR/单元测试" ;;
        emulator|device|live-emulator|live-device)
            ADB="${ANDROID_HOME:-${ANDROID_SDK_ROOT:-$HOME/Library/Android/sdk}}/platform-tools/adb"
            [[ -x "$ADB" ]] || { print -u2 '缺Android platform-tools/adb'; return 2; }
            choose_target "${mode#live-}" || return $?
            local -a live_args
            live_args=()
            if [[ "$mode" == live-* ]]; then
                [[ -n "${JOBS_IM_TEST_URL:-}" && -n "${JOBS_IM_DEPLOYMENT_ID:-}" && -f "${JOBS_IM_CA_FILE:-}" ]] || { print -u2 "live模式需要JOBS_IM_TEST_URL/JOBS_IM_DEPLOYMENT_ID/JOBS_IM_CA_FILE，且仅面向隔离测试服务器。"; return 2; }
                live_args+=("-Pandroid.testInstrumentationRunnerArguments.liveUrl=$JOBS_IM_TEST_URL" "-Pandroid.testInstrumentationRunnerArguments.deploymentId=$JOBS_IM_DEPLOYMENT_ID" "-Pandroid.testInstrumentationRunnerArguments.caBase64=$(base64 < "$JOBS_IM_CA_FILE" | tr -d '\n')")
            fi
            print "测试模式=$mode 目标=$TARGET"
            ANDROID_SERIAL="$TARGET" "$ANDROID_PROJECT/gradlew" -p "$ANDROID_PROJECT" --no-daemon :app:connectedDebugAndroidTest "${live_args[@]}" || return $?
            cp -R "$ANDROID_PROJECT/app/build/outputs/androidTest-results/connected/debug" "$REPORT_DIR/设备测试"
            "$ADB" -s "$TARGET" install -r "$ANDROID_PROJECT/app/build/outputs/apk/debug/app-debug.apk" || return $?
            "$ADB" -s "$TARGET" shell am start -W -n com.jobs.im/.MainActivity || return $?
            "$ADB" -s "$TARGET" exec-out screencap -p > "$REPORT_DIR/启动截图.png" || return $?
            "$ADB" -s "$TARGET" shell getprop ro.build.version.release > "$REPORT_DIR/系统版本.txt"
            ;;
        *) print -u2 '用法：运行测试.command check|unit|build|emulator|device|live-emulator|live-device'; return 2 ;;
    esac
}
main() {
    initialize
    local mode="${1:-check}"
    print 'JobsIM Android测试：check只检查；unit/build编译并运行逻辑测试；emulator/device会在所选设备安装测试App并运行Room隔离测试。Ctrl+C取消。'
    local result=0
    run_tests "$mode" 2>&1 | tee "$REPORT_DIR/运行日志.txt" || result=$?
    print "退出码=$result；日志=$REPORT_DIR"
    return "$result"
}
main "$@"
