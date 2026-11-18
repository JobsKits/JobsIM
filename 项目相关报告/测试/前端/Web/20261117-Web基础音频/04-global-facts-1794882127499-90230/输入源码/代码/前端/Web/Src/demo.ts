import type { Space, User } from "./types";
export const demoUser: User = {
  id: "demo-me",
  username: "jobs",
  displayName: "Jobs",
};
export const demoUsers: User[] = [
  demoUser,
  { id: "demo-lin", username: "lin", displayName: "林间" },
  { id: "demo-yu", username: "yu", displayName: "予安" },
];
export function demoSpace(): Space {
  const now = new Date();
  const date = (minutes: number) =>
    new Date(now.getTime() - minutes * 60000).toISOString();
  const c = [
    {
      id: "demo-design",
      kind: "group" as const,
      title: "产品设计讨论",
      ownerId: "demo-me",
      memberIds: demoUsers.map((x) => x.id),
      updatedAt: date(2),
      cursor: "5",
    },
    {
      id: "demo-lin",
      kind: "direct" as const,
      title: "林间",
      ownerId: "demo-me",
      memberIds: ["demo-me", "demo-lin"],
      updatedAt: date(27),
      cursor: "2",
    },
    {
      id: "demo-notes",
      kind: "self" as const,
      title: "我的笔记",
      ownerId: "demo-me",
      memberIds: ["demo-me"],
      updatedAt: date(90),
      cursor: "1",
    },
  ];
  const messages: any = {};
  const add = (
    conversationId: string,
    senderId: string,
    text: string,
    minutes: number,
    pinned = false,
  ) => {
    const sequence = String((messages[conversationId]?.length ?? 0) + 1);
    const m = {
      id: conversationId + "-" + sequence,
      conversationId,
      senderId,
      clientMsgId: crypto.randomUUID(),
      type: "text",
      text,
      createdAt: date(minutes),
      updatedAt: date(minutes),
      version: "1",
      sequence,
      deleted: false,
      reactions: [],
      pinned,
      favorite: false,
    };
    (messages[conversationId] ??= []).push(m);
  };
  add(
    "demo-design",
    "demo-lin",
    "早上好，我们把消息页面的第一轮体验整理好了。",
    1488,
  );
  add(
    "demo-design",
    "demo-me",
    "让会话和消息成为主角。日常聊天要轻松，重要的状态也要说清楚。",
    1484,
    true,
  );
  add(
    "demo-design",
    "demo-yu",
    "赞同。断网时也要保留草稿，恢复连接后再继续。",
    39,
  );
  add(
    "demo-design",
    "demo-me",
    "浅色、深色、跟随系统都安排上。今天先把基础体验打磨好。",
    18,
  );
  add("demo-design", "demo-lin", "收到，我来回归多语言和小屏幕的表现。", 2);
  add("demo-lin", "demo-lin", "周末去走走？我发现一条很安静的山间步道。", 30);
  add("demo-lin", "demo-me", "好呀，晚一点一起看看路线。", 27);
  add("demo-notes", "demo-me", "把灵感留在这里，想到什么就记下来。", 90);
  return { conversations: c, messages, cursors: {}, drafts: {}, outbox: [] };
}
