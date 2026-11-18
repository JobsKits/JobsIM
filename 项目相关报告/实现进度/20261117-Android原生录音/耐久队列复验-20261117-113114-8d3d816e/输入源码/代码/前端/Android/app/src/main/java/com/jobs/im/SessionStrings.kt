package com.jobs.im

object SessionStrings {
    private val keys = "verifySession|authUnknown|authRetryOriginal|authProofHelp|authVerifyNew".split('|')
    private val rows = mapOf(
        "zh-Hans" to "请重新验证身份，原待发已保留|上次认证结果尚未确认，请重输原凭据重试，或重新验证并结束旧认证|重试原认证|原重试需要相同密码和验证码。使用新验证码时请选择重新验证；未完成的注册仍可沿原认证重试。|重新验证并结束旧认证",
        "zh-Hant" to "請重新驗證身分，原待發已保留|上次認證結果尚未確認，請重輸原憑證重試，或重新驗證並結束舊認證|重試原認證|原重試需要相同密碼和驗證碼。使用新驗證碼時請重新驗證；未完成的註冊仍可沿原認證重試。|重新驗證並結束舊認證",
        "en" to "Verify your identity again. Your original outbox is preserved|The last sign-in result is unknown. Retry with the original credentials, or verify again to end the previous sign-in|Retry original authentication|An original retry needs the same password and code. For a new code, verify again. An unfinished registration can still retry its original request.|Verify again and end previous sign-in",
        "ja" to "本人確認をやり直してください。元の送信待ちは保持されています|前回の認証結果は不明です。同じ情報で再試行するか、本人確認をして前の認証を終了してください|元の認証を再試行|再試行には同じパスワードとコードが必要です。新しいコードは本人確認を選択してください。未完了の登録は元のリクエストを再試行できます。|本人確認して前の認証を終了",
        "fr" to "Vérifiez à nouveau votre identité. La file initiale est conservée|Le résultat précédent est inconnu. Réessayez les mêmes identifiants ou vérifiez votre identité pour terminer la connexion précédente|Réessayer l’authentification initiale|Utilisez le même mot de passe et le même code pour réessayer. Avec un nouveau code, vérifiez à nouveau. Une inscription inachevée peut reprendre sa requête initiale.|Vérifier et terminer la connexion précédente",
        "es" to "Verifica tu identidad de nuevo. La cola original se conserva|El resultado anterior es desconocido. Repite las credenciales originales o verifica de nuevo para cerrar el acceso anterior|Reintentar autenticación original|El reintento requiere la misma contraseña y código. Con un código nuevo, verifica de nuevo. Un registro incompleto puede repetir su solicitud original.|Verificar y cerrar el acceso anterior",
        "de" to "Identität erneut bestätigen. Der ursprüngliche Postausgang bleibt erhalten|Das letzte Ergebnis ist unbekannt. Ursprüngliche Zugangsdaten wiederholen oder erneut bestätigen, um die vorherige Anmeldung zu beenden|Ursprüngliche Anmeldung wiederholen|Für die Wiederholung sind dasselbe Passwort und derselbe Code nötig. Bei neuem Code erneut bestätigen. Eine unvollständige Registrierung kann ihre ursprüngliche Anfrage wiederholen.|Bestätigen und vorherige Anmeldung beenden",
        "ru" to "Подтвердите личность снова. Исходная очередь сохранена|Результат прошлой авторизации неизвестен. Повторите исходные данные или подтвердите личность, чтобы завершить прошлый вход|Повторить исходную авторизацию|Нужны прежние пароль и код. С новым кодом подтвердите личность заново. Незавершённая регистрация может повторить исходный запрос.|Подтвердить и завершить прошлый вход",
        "ko" to "본인 확인이 필요합니다. 원래 전송 대기열은 보존됩니다|이전 인증 결과를 알 수 없습니다. 원래 정보로 재시도하거나 다시 확인하여 이전 인증을 종료하세요|원래 인증 재시도|재시도에는 같은 비밀번호와 코드가 필요합니다. 새 코드라면 다시 확인하세요. 미완료 가입은 원래 요청을 재시도할 수 있습니다.|다시 확인하고 이전 인증 종료",
        "bo" to "རང་ཉིད་ཡང་བསྐྱར་བདེན་དཔང་བྱ། མ་སྐུར་བ་ཉར་ཡོད།|སྔོན་གྱི་བདེན་དཔང་འབྲས་བུ་མ་ངེས། མ་གསང་ཨང་གིས་ཡང་འབད་ཡང་ན་སྔོན་མ་མཇུག་སྒྲིལ།|མ་བདེན་དཔང་ཡང་འབད།|ཡང་འབད་ལ་གསང་ཨང་དང་ཨང་རྙིང་པ་དགོས། ཨང་གསར་པ་ཡིན་ན་བདེན་དཔང་གསར་བྱ། ཐོ་འགོད་མ་གྲུབ་པ་མ་རེ་ཞུས་ཡང་འབད་ཆོག|ཡང་དཔང་བྱས་ནས་སྔོན་མ་མཇུག་སྒྲིལ།",
    ).mapValues { it.value.split('|') }

    fun text(language: String, key: String): String? =
        keys.indexOf(key).takeIf { it >= 0 }?.let { rows[language]?.getOrNull(it) }

    fun complete() = Strings.languages.keys.all { rows[it]?.size == keys.size }
}
