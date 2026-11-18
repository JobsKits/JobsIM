// Created by Jobs on 2026年11月18日，星期三.
const keys = ["reportDetail", "reporterId", "targetUserId", "reportEvidence", "selectedReportMessages", "reportScopeNote", "viewEvidence", "noReportEvidence", "filterReports", "clearReportFilters", "reportPreviewLimit"];
const rows: Record<string, string[]> = {
  "zh-CN": [
    "举报详情",
    "举报人ID",
    "被举报人ID",
    "举报证据",
    "所选聊天记录",
    "仅显示举报人主动选择的材料，不包含整段会话。",
    "查看证据",
    "暂无提交材料",
    "筛选举报",
    "清除筛选",
    "此浏览器支持图片不超过20MiB、视频不超过64MiB。"
  ],
  "zh-TW": [
    "檢舉詳情",
    "檢舉人ID",
    "被檢舉人ID",
    "檢舉證據",
    "所選聊天記錄",
    "僅顯示檢舉人主動選擇的材料，不包含整段對話。",
    "查看證據",
    "尚無提交材料",
    "篩選檢舉",
    "清除篩選",
    "此瀏覽器支援圖片不超過20MiB、影片不超過64MiB。"
  ],
  "bo": [
    "སྙན་ཞུའི་ཞིབ་ཕྲ།",
    "སྙན་ཞུ་མཁན་ཨང་།",
    "སྙན་ཞུ་བྱ་ཡུལ་ཨང་།",
    "སྙན་ཞུའི་དཔང་རྟགས།",
    "བདམས་པའི་ཁ་བརྡ།",
    "སྙན་ཞུ་མཁན་གྱིས་བདམས་པའི་རྒྱུ་ཆ་ཁོ་ན་སྟོན། ཁ་བརྡ་ཆ་ཚང་མི་ཚུད།",
    "དཔང་རྟགས་ལྟ།",
    "རྒྱུ་ཆ་མ་ཕུལ།",
    "སྙན་ཞུ་འཚག",
    "འཚག་ཆས་གཙང་སེལ།",
    "བཤར་ཆས་འདིར་པར་20MiB་མན་དང་བརྙན་64MiB་མན་སྔོན་ལྟ་བྱེད།"
  ],
  "en": [
    "Report Details",
    "Reporter ID",
    "Reported User ID",
    "Report Evidence",
    "Selected Chat Messages",
    "Only material selected by the reporter is included, never the whole conversation.",
    "View Evidence",
    "No material submitted",
    "Filter Reports",
    "Clear Filters",
    "This browser supports images up to 20MiB and videos up to 64MiB."
  ],
  "ja": [
    "通報の詳細",
    "通報者ID",
    "通報対象のユーザーID",
    "通報資料",
    "選択されたメッセージ",
    "通報者が選んだ資料のみを表示します。会話全体は含みません。",
    "資料を表示",
    "提出された資料はありません",
    "通報を絞り込む",
    "絞り込みを解除",
    "このブラウザでは画像は20MiB、動画は64MiBまで表示できます。"
  ],
  "fr": [
    "Détails du signalement",
    "ID du signalant",
    "ID de l’utilisateur signalé",
    "Pièces du signalement",
    "Messages sélectionnés",
    "Seules les pièces choisies par le signalant sont incluses, jamais la conversation entière.",
    "Voir les pièces",
    "Aucune pièce soumise",
    "Filtrer les signalements",
    "Effacer les filtres",
    "Ce navigateur affiche les images jusqu’à 20MiB et les vidéos jusqu’à 64MiB."
  ],
  "es": [
    "Detalles de la denuncia",
    "ID del denunciante",
    "ID del usuario denunciado",
    "Pruebas de la denuncia",
    "Mensajes seleccionados",
    "Solo se incluye el material seleccionado, nunca la conversación completa.",
    "Ver pruebas",
    "No se aportaron pruebas",
    "Filtrar denuncias",
    "Borrar filtros",
    "Este navegador muestra imágenes de hasta 20MiB y vídeos de hasta 64MiB."
  ],
  "de": [
    "Meldungsdetails",
    "ID der meldenden Person",
    "ID der gemeldeten Person",
    "Meldebelege",
    "Ausgewählte Chatnachrichten",
    "Nur ausgewähltes Material wird angezeigt, niemals das gesamte Gespräch.",
    "Belege ansehen",
    "Keine Belege eingereicht",
    "Meldungen filtern",
    "Filter löschen",
    "Dieser Browser zeigt Bilder bis 20MiB und Videos bis 64MiB an."
  ],
  "ru": [
    "Сведения о жалобе",
    "ID заявителя",
    "ID пользователя в жалобе",
    "Материалы жалобы",
    "Выбранные сообщения",
    "Включены только выбранные заявителем материалы, без полной переписки.",
    "Посмотреть материалы",
    "Материалы не приложены",
    "Фильтр жалоб",
    "Сбросить фильтры",
    "В браузере доступны изображения до 20MiB и видео до 64MiB."
  ],
  "ko": [
    "신고 상세",
    "신고자 ID",
    "신고 대상 사용자 ID",
    "신고 증거",
    "선택한 채팅 메시지",
    "신고자가 선택한 자료만 포함되며 전체 대화는 포함되지 않습니다.",
    "증거 보기",
    "제출된 자료 없음",
    "신고 필터",
    "필터 지우기",
    "이 브라우저에서는 이미지 20MiB, 동영상 64MiB까지 미리 볼 수 있습니다."
  ]
};
export const adminReportLabels: Record<string, Record<string,string>> = Object.fromEntries(Object.entries(rows).map(([locale, values]) => {
  if (values.length !== keys.length) throw Error("ADMIN_REPORT_TRANSLATION_INVALID:" + locale);
  return [locale, Object.fromEntries(keys.map((key,index) => [key,values[index]]))];
}));
