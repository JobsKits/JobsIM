// Created by Jobs on 2026年11月16日，星期一.
export class SyncText {
  static text(language: string, key: string): string {
    const keys = ['restricted', 'local', 'localOnly'];
    const languages = ['zh-Hans', 'zh-Hant', 'bo', 'en', 'ja', 'fr', 'es', 'de', 'ru', 'ko'];
    const rows = [
      ['内容待重新验证。原队列和合法本地历史已保留，请重新加载。', '合法本地历史', '仅本地只读；不取得云端、附件或发送权限。'],
      ['內容待重新驗證。原佇列與合法本機歷史已保留，請重新載入。', '合法本機歷史', '僅本機唯讀；不取得雲端、附件或傳送權限。'],
      ['ནང་དོན་བསྐྱར་དུ་ཞིབ་བཤེར་དགོས། ས་གནས་ཀྱི་ལོ་རྒྱུས་ཉར་ཡོད།', 'ས་གནས་ཀྱི་ལོ་རྒྱུས།', 'ས་གནས་ནས་ཀློག་པ་ཙམ་ཡིན། གཏོང་མི་ཐུབ།'],
      ['Content needs verification. Original queues and lawful local history are preserved. Reload to retry.', 'Lawful local history', 'Local read only; no cloud, attachment or sending access.'],
      ['内容の再確認が必要です。元の送信待ちと正当に保持した履歴は保存されています。再読み込みしてください。', '保持済みのローカル履歴', 'ローカル閲覧のみ。クラウド、添付、送信の権限はありません。'],
      ['Le contenu doit être vérifié. Les envois en attente et l’historique local autorisé sont conservés. Rechargez.', 'Historique local autorisé', 'Lecture locale seule ; aucun accès cloud, aux pièces jointes ou à l’envoi.'],
      ['Hay que verificar el contenido. Se conservan la cola original y el historial local autorizado. Recarga.', 'Historial local autorizado', 'Solo lectura local; sin acceso a la nube, adjuntos ni envío.'],
      ['Der Inhalt muss geprüft werden. Ursprüngliche Warteschlange und berechtigter lokaler Verlauf bleiben erhalten. Neu laden.', 'Berechtigter lokaler Verlauf', 'Nur lokal lesen; kein Cloud-, Anhangs- oder Sendezugriff.'],
      ['Содержимое требует проверки. Исходная очередь и законно сохранённая история остаются. Обновите.', 'Сохранённая локальная история', 'Только локальное чтение; без облака, вложений и отправки.'],
      ['내용을 다시 확인해야 합니다. 원래 대기열과 적법하게 보관한 로컬 기록은 유지됩니다. 새로고침하세요.', '보관된 로컬 기록', '로컬 읽기 전용이며 클라우드·첨부·전송 권한은 없습니다.']
    ];
    return rows[Math.max(0, languages.indexOf(language))][Math.max(0, keys.indexOf(key))];
  }
}
