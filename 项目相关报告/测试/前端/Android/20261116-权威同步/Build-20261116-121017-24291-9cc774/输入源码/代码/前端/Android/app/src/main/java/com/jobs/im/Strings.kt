package com.jobs.im

object Strings {
    val languages =
        linkedMapOf(
            "zh-Hans" to "简体中文",
            "zh-Hant" to "繁體中文",
            "bo" to "བོད་ཡིག",
            "en" to "English",
            "ja" to "日本語",
            "fr" to "Français",
            "es" to "Español",
            "de" to "Deutsch",
            "ru" to "Русский",
            "ko" to "한국어",
        )
    private val keys =
        "chats|contacts|settings|connect|login|register|recover|username|password|code|continue|demo|demoNote|offline|refresh|search|empty|emptyHelp|send|message|back|newChat|group|title|members|create|reply|edit|retract|react|pin|favorite|retry|cancel|save|language|theme|system|light|dark|font|small|normal|large|logout|server|expected|verified|attachment|retracted|pending|codes|clear|trust|ca|fingerprint|trustHelp|close|add|newPassword|connectionHelp|welcome|private|status|copied|forward"
            .split('|')
    private val translations =
        mapOf(
                "zh-Hans" to
                    "会话|联系人|设置|连接服务器|登录|注册|恢复账号|用户名|密码|恢复码|继续|本地演示|演示数据仅保存在本机，不会发往服务器|离线，正式待发已保留|重新加载|搜索|这里还没有内容|重新加载，或开始新的会话|发送|输入消息|返回|发起聊天|群聊|名称|成员 ID，以逗号分隔|创建|回复|编辑|撤回|回应|置顶|收藏|重试|取消|保存|语言|主题|跟随系统|白天|黑夜|字号|小|标准|大|退出登录|服务地址|部署实例 ID|已核对实例|附件|消息已撤回|待确认|生成恢复码|清空|核对并信任|导入本地 CA|证书指纹|请与可信部署终端核对指纹；仅信任此实例|关闭|添加好友|新密码|先核对服务器身份，再输入账号密码|让对话自然发生|自己的服务器，自己的联系|连接状态|已复制|转发",
                "zh-Hant" to
                    "會話|聯絡人|設定|連接伺服器|登入|註冊|恢復帳號|使用者名稱|密碼|恢復碼|繼續|本機示範|示範資料只儲存在本機，不會傳送至伺服器|離線，正式待發已保留|重新載入|搜尋|這裡還沒有內容|重新載入，或開始新的會話|傳送|輸入訊息|返回|發起聊天|群組|名稱|成員 ID，以逗號分隔|建立|回覆|編輯|收回|回應|置頂|收藏|重試|取消|儲存|語言|主題|跟隨系統|日間|夜間|字級|小|標準|大|登出|服務位址|部署實例 ID|已核對實例|附件|訊息已收回|待確認|產生恢復碼|清空|核對並信任|匯入本機 CA|憑證指紋|請與可信部署終端核對指紋；僅信任此實例|關閉|新增好友|新密碼|先核對伺服器身分，再輸入帳號密碼|讓對話自然發生|自己的伺服器，自己的聯絡|連線狀態|已複製|轉寄",
                "en" to
                    "Chats|Contacts|Settings|Connect server|Sign in|Create account|Recover account|Username|Password|Recovery code|Continue|Local preview|Preview stays on this device and never reaches your server|Offline. Your live outbox is preserved|Reload|Search|Nothing here yet|Reload or start a conversation|Send|Write a message|Back|New chat|Group|Name|Member IDs, comma separated|Create|Reply|Edit|Retract|React|Pin|Save|Retry|Cancel|Save|Language|Theme|System|Light|Dark|Text size|Small|Standard|Large|Sign out|Server address|Deployment ID|Verified instance|Attachment|Message retracted|Unconfirmed|Generate recovery codes|Clear|Verify and trust|Import local CA|Certificate fingerprint|Compare with your trusted deployment terminal; trust applies only to this instance|Close|Add contact|New password|Verify the server before entering your credentials|Room for conversation|Your server. Your connections.|Connection status|Copied|Forward",
                "ja" to
                    "チャット|連絡先|設定|サーバー接続|ログイン|アカウント作成|アカウント復旧|ユーザー名|パスワード|復旧コード|続行|ローカルデモ|デモは端末内だけに保存されサーバーに送信されません|オフライン。送信待ちは保持されます|再読込|検索|まだありません|再読込または会話を開始|送信|メッセージを入力|戻る|新しい会話|グループ|名前|メンバーIDをカンマで区切る|作成|返信|編集|撤回|反応|固定|保存|再試行|キャンセル|保存|言語|テーマ|システム|ライト|ダーク|文字サイズ|小|標準|大|ログアウト|サーバーURL|配置ID|確認済みサーバー|添付|撤回されたメッセージ|未確認|復旧コード生成|クリア|確認して信頼|ローカルCAを取込|証明書の指紋|信頼できる配置端末と指紋を比較。このサーバーだけに適用|閉じる|連絡先を追加|新パスワード|認証情報の入力前にサーバーを確認|会話が生まれる場所|自分のサーバー、自分のつながり|接続状態|コピー済み|転送",
                "fr" to
                    "Discussions|Contacts|Réglages|Connecter un serveur|Connexion|Créer un compte|Récupérer le compte|Identifiant|Mot de passe|Code de récupération|Continuer|Aperçu local|La démonstration reste sur cet appareil et ne rejoint jamais le serveur|Hors ligne. La file réelle est conservée|Actualiser|Rechercher|Aucun contenu|Actualisez ou démarrez une discussion|Envoyer|Écrire un message|Retour|Nouvelle discussion|Groupe|Nom|Identifiants séparés par des virgules|Créer|Répondre|Modifier|Retirer|Réagir|Épingler|Enregistrer|Réessayer|Annuler|Enregistrer|Langue|Thème|Système|Clair|Sombre|Taille du texte|Petite|Normale|Grande|Déconnexion|Adresse du serveur|ID du déploiement|Instance vérifiée|Pièce jointe|Message retiré|Non confirmé|Créer les codes de récupération|Effacer|Vérifier et approuver|Importer une AC locale|Empreinte du certificat|Comparez avec le terminal de déploiement fiable ; confiance limitée à cette instance|Fermer|Ajouter un contact|Nouveau mot de passe|Vérifiez le serveur avant de saisir vos identifiants|Place à la conversation|Votre serveur. Vos liens.|État de connexion|Copié|Transférer",
                "es" to
                    "Chats|Contactos|Ajustes|Conectar servidor|Iniciar sesión|Crear cuenta|Recuperar cuenta|Usuario|Contraseña|Código de recuperación|Continuar|Vista local|La demo permanece en este dispositivo y nunca llega al servidor|Sin conexión. La cola real se conserva|Actualizar|Buscar|Sin contenido|Actualiza o inicia una conversación|Enviar|Escribe un mensaje|Volver|Nuevo chat|Grupo|Nombre|IDs separados por comas|Crear|Responder|Editar|Retirar|Reaccionar|Fijar|Guardar|Reintentar|Cancelar|Guardar|Idioma|Tema|Sistema|Claro|Oscuro|Tamaño de texto|Pequeño|Normal|Grande|Cerrar sesión|Dirección del servidor|ID de despliegue|Instancia verificada|Adjunto|Mensaje retirado|Sin confirmar|Generar códigos de recuperación|Borrar|Verificar y confiar|Importar CA local|Huella del certificado|Compara con el terminal fiable; confianza solo para esta instancia|Cerrar|Añadir contacto|Nueva contraseña|Verifica el servidor antes de introducir credenciales|Un lugar para conversar|Tu servidor. Tus conexiones.|Estado de conexión|Copiado|Reenviar",
                "de" to
                    "Chats|Kontakte|Einstellungen|Server verbinden|Anmelden|Konto erstellen|Konto wiederherstellen|Benutzername|Passwort|Wiederherstellungscode|Weiter|Lokale Vorschau|Die Demo bleibt auf diesem Gerät und wird nie übertragen|Offline. Der echte Postausgang bleibt erhalten|Neu laden|Suchen|Noch keine Inhalte|Neu laden oder Gespräch beginnen|Senden|Nachricht schreiben|Zurück|Neuer Chat|Gruppe|Name|Mitglieds-IDs durch Kommas trennen|Erstellen|Antworten|Bearbeiten|Zurückziehen|Reagieren|Anheften|Speichern|Wiederholen|Abbrechen|Speichern|Sprache|Design|System|Hell|Dunkel|Textgröße|Klein|Standard|Groß|Abmelden|Serveradresse|Bereitstellungs-ID|Geprüfte Instanz|Anhang|Nachricht zurückgezogen|Unbestätigt|Wiederherstellungscodes erzeugen|Leeren|Prüfen und vertrauen|Lokale CA importieren|Zertifikatsfingerabdruck|Mit dem vertrauenswürdigen Bereitstellungsterminal vergleichen; nur für diese Instanz|Schließen|Kontakt hinzufügen|Neues Passwort|Server vor der Eingabe der Zugangsdaten prüfen|Raum für Gespräche|Ihr Server. Ihre Kontakte.|Verbindungsstatus|Kopiert|Weiterleiten",
                "ru" to
                    "Чаты|Контакты|Настройки|Подключить сервер|Войти|Создать аккаунт|Восстановить аккаунт|Имя пользователя|Пароль|Код восстановления|Продолжить|Локальный пример|Демо хранится только на устройстве и не отправляется на сервер|Нет сети. Реальная очередь сохранена|Обновить|Поиск|Пока пусто|Обновите или начните разговор|Отправить|Написать сообщение|Назад|Новый чат|Группа|Название|ID участников через запятую|Создать|Ответить|Изменить|Отозвать|Реакция|Закрепить|Сохранить|Повторить|Отмена|Сохранить|Язык|Тема|Системная|Светлая|Тёмная|Размер текста|Малый|Обычный|Большой|Выйти|Адрес сервера|ID развёртывания|Проверенный экземпляр|Вложение|Сообщение отозвано|Не подтверждено|Создать коды восстановления|Очистить|Проверить и доверять|Импорт локального CA|Отпечаток сертификата|Сверьте с доверенным терминалом; доверие только этому экземпляру|Закрыть|Добавить контакт|Новый пароль|Проверьте сервер до ввода учётных данных|Место для разговора|Ваш сервер. Ваши контакты.|Состояние подключения|Скопировано|Переслать",
                "ko" to
                    "채팅|연락처|설정|서버 연결|로그인|계정 만들기|계정 복구|사용자 이름|비밀번호|복구 코드|계속|로컬 미리보기|데모는 기기에만 저장되며 서버로 전송되지 않습니다|오프라인. 실제 전송 대기열은 보존됩니다|새로고침|검색|아직 내용이 없습니다|새로고침하거나 대화를 시작하세요|보내기|메시지 입력|뒤로|새 채팅|그룹|이름|쉼표로 구분한 구성원 ID|만들기|답장|편집|회수|반응|고정|저장|재시도|취소|저장|언어|테마|시스템|밝게|어둡게|글자 크기|작게|표준|크게|로그아웃|서버 주소|배포 ID|확인된 인스턴스|첨부|회수된 메시지|미확인|복구 코드 생성|지우기|확인 후 신뢰|로컬 CA 가져오기|인증서 지문|신뢰할 수 있는 배포 터미널과 비교하세요. 이 인스턴스에만 적용됩니다|닫기|연락처 추가|새 비밀번호|자격 증명을 입력하기 전에 서버를 확인하세요|대화를 위한 공간|내 서버. 내 연결.|연결 상태|복사됨|전달",
                "bo" to
                    "ཁ་བརྡ།|འབྲེལ་མི།|སྒྲིག་འགོད།|ཞབས་ཞུ་སྦྲེལ།|ནང་འཇུག|ཐོ་འགོད།|རྩིས་ཁྲ་སླར་གསོ།|སྤྱོད་མིའི་མིང་།|གསང་ཨང་།|སླར་གསོའི་ཨང་།|མུ་མཐུད།|རང་སའི་དཔེ་སྟོན།|དཔེ་སྟོན་གཞི་གྲངས་འཕྲུལ་ཆས་འདིར་ཁོ་ན་ཉར།|དྲ་མེད། སྐུར་རྒྱུ་ཉར་ཡོད།|ཡང་བསྐྱར་འཇུག|འཚོལ།|ད་དུང་མེད།|ཡང་བསྐྱར་འཇུག་ཡང་ན་ཁ་བརྡ་གསར་པ།|སྐུར།|འཕྲིན་ཡིག་འབྲི།|ཕྱིར་ལོག|ཁ་བརྡ་གསར་པ།|ཚོགས་པ།|མིང་།|ཚོགས་མིའི་ID|གསར་བཟོ།|ལན།|བཟོ་བཅོས།|ཕྱིར་འཐེན།|ལན་སྤྲོད།|རྩེར་འཇོག|ཉར།|ཡང་བསྐྱར།|མེད་པར་བཟོ།|ཉར།|སྐད་ཡིག|བརྗོད་གཞི།|མ་ལག་བསྟུན།|ཉིན།|མཚན།|ཡིག་གཟུགས་ཆེ་ཆུང་།|ཆུང་།|ཚད་ལྡན།|ཆེ།|ཕྱིར་ཐོན།|ཞབས་ཞུའི་དྲ་གནས།|བཀོད་སྒྲིག་ID|བདེན་དཔང་ཟིན།|ཟུར་སྣོན།|འཕྲིན་ཕྱིར་འཐེན་ཟིན།|ངེས་མེད།|སླར་གསོའི་ཨང་གསར་བཟོ།|གཙང་སེལ།|བདེན་དཔང་དང་ཡིད་ཆེས།|རང་སའི་CA་ནང་འདྲེན།|དཔང་ཡིག་མཛུབ་རྟགས།|ཡིད་ཆེས་བཀོད་སྒྲིག་འཕྲུལ་ཆས་དང་བསྡུར།|ཁ་རྒྱག|འབྲེལ་མི་སྣོན།|གསང་ཨང་གསར་པ།|ཞབས་ཞུ་བདེན་དཔང་བྱས་ནས་གསང་ཨང་འཇུག|ཁ་བརྡའི་གནས།|རང་གི་ཞབས་ཞུ། རང་གི་འབྲེལ་མི།|སྦྲེལ་མཐུད་གནས་ཚུལ།|འདྲ་བཤུས་ཟིན།|བརྒྱུད་སྐུར།",
            )
            .mapValues { it.value.split('|') }

    fun text(language: String, key: String): String {
        SessionStrings.text(language, key)?.let { return it }
        FeatureStrings.text(language, key)?.let {
            return it
        }
        ExtraStrings.text(language, key)?.let {
            return it
        }
        val index = keys.indexOf(key)
        return translations[language]?.getOrNull(index)
            ?: translations.getValue("en").getOrNull(index)
            ?: key
    }

    fun complete(): Boolean =
        languages.keys.all { translations[it]?.size == keys.size } &&
            ExtraStrings.complete() &&
            FeatureStrings.complete() && SessionStrings.complete()
}
