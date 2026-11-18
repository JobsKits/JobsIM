// Created by Jobs on 2026年11月15日，星期日.
import { useEffect, useRef, useState } from "react";
import {
  Bookmark,
  ChevronRight,
  MessageCircle,
  RefreshCw,
  Search,
} from "lucide-react";
import { api, ApiError } from "./api";
import { type Message } from "./types";
import { unavailableForward } from "./forward";

type Page = { items: Message[]; cursor: string; hasMore: boolean };
export default function MessageLookup({
  kind,
  demo,
  cached,
  conversationId,
  filter = "",
  t,
  text,
  date,
  onChoose,
  onError,
}: {
  kind: "search" | "favorites";
  demo: boolean;
  cached: Message[];
  conversationId?: string;
  filter?: string;
  t: (key: string) => string;
  text: (message: Message) => string;
  date: (value: string) => string;
  onChoose: (message: Message) => Promise<void>;
  onError: (error: unknown) => void;
}) {
  const [items, setItems] = useState<Message[]>(
      kind === "favorites"
        ? cached.filter((message) => message.favorite && !message.deleted)
        : [],
    ),
    [query, setQuery] = useState(""),
    [submitted, setSubmitted] = useState<string | null>(
      kind === "favorites" ? "" : null,
    ),
    [cursor, setCursor] = useState(""),
    [more, setMore] = useState(false),
    [busy, setBusy] = useState(false);
  const sequence = useRef(0),
    alive = useRef(true),
    working = useRef(false),
    itemsRef = useRef(items),
    forwardOffset = useRef(0);
  itemsRef.current = items;
  const load = async (value: string, append = false) => {
    if (working.current) return;
    if (kind === "search" && !value.trim()) return;
    const request = ++sequence.current;
    working.current = true;
    setBusy(true);
    setSubmitted(value);
    try {
      let result: Page;
      if (demo)
        result = {
          items: cached.filter(
            (message) =>
              !message.deleted &&
              (kind === "favorites"
                ? message.favorite
                : message.text.includes(value)),
          ),
          cursor: "",
          hasMore: false,
        };
      else {
        const params = new URLSearchParams({ limit: "50" });
        if (kind === "search") {
          params.set("q", value);
          if (conversationId) params.set("conversationId", conversationId);
        }
        if (append && cursor) params.set("cursor", cursor);
        result = await api<Page>("/" + kind + "?" + params);
        if (
          !Array.isArray(result.items) ||
          typeof result.cursor !== "string" ||
          typeof result.hasMore !== "boolean"
        )
          throw Error("Invalid message page");
      }
      if (!alive.current || request !== sequence.current) return;
      setItems((previous) =>
        append
          ? [
              ...new Map(
                [...previous, ...result.items].map((message) => [
                  message.id,
                  message,
                ]),
              ).values(),
            ]
          : result.items,
      );
      setCursor(result.cursor);
      setMore(result.hasMore);
    } catch (error) {
      if (alive.current && request === sequence.current) {
        setItems((rows) =>
          rows.map((message) =>
            message.forwarded ? unavailableForward(message) : message,
          ),
        );
        if (
          error instanceof ApiError &&
          [401, 403, 404, 409].includes(error.status)
        ) {
          setItems([]);
          setMore(false);
          setCursor("");
        }
        onError(error);
      }
    } finally {
      working.current = false;
      if (alive.current && request === sequence.current) setBusy(false);
    }
  };
  useEffect(() => {
    alive.current = true;
    if (kind === "favorites") void load("");
    return () => {
      alive.current = false;
      sequence.current++;
    };
  }, []);
  useEffect(() => {
    if (demo) return;
    let running = false;
    const timer = setInterval(async () => {
      if (running || working.current) return;
      const forwards = itemsRef.current.filter(
        (message) => message.forwarded && !message.deleted,
      );
      if (!forwards.length) return;
      running = true;
      const request = sequence.current;
      const start = forwardOffset.current % forwards.length;
      const batch = [
        ...forwards.slice(start),
        ...forwards.slice(0, start),
      ].slice(0, 20);
      forwardOffset.current = start + batch.length;
      try {
        await Promise.all(
          batch.map(async (cached) => {
            let projection: Message;
            try {
              projection = await api<Message>(
                "/messages/" + encodeURIComponent(cached.id),
              );
            } catch {
              projection = unavailableForward(cached);
            }
            if (!alive.current || request !== sequence.current) return;
            setItems((rows) =>
              rows.map((old) =>
                old.id !== cached.id ||
                BigInt(old.version) > BigInt(projection.version)
                  ? old
                  : projection,
              ),
            );
          }),
        );
      } finally {
        running = false;
      }
    }, 3000);
    return () => clearInterval(timer);
  }, [demo]);
  const choose = async (message: Message) => {
    try {
      await onChoose(message);
    } catch (error) {
      if (alive.current) {
        if (message.forwarded) sequence.current++;
        if (message.forwarded)
          setItems((rows) =>
            rows.map((old) =>
              old.id === message.id ? unavailableForward(old) : old,
            ),
          );
        if (error instanceof ApiError && [401, 403, 404].includes(error.status))
          setItems((rows) => rows.filter((item) => item.id !== message.id));
        onError(error);
      }
    }
  };
  const visible = items.filter(
    (message) =>
      !filter ||
      text(message).toLocaleLowerCase().includes(filter.toLocaleLowerCase()),
  );
  return (
    <>
      {kind === "search" ? (
        <form
          onSubmit={(event) => {
            event.preventDefault();
            void load(query);
          }}
        >
          <input
            name="q"
            required
            aria-label={t("search")}
            value={query}
            onChange={(event) => setQuery(event.target.value)}
          />
          <button className="primary" disabled={busy}>
            {t(busy ? "working" : "search")}
          </button>
        </form>
      ) : (
        <div className="report-heading lookup-heading">
          <h3>{t("favorites")}</h3>
          <button
            className="icon-button"
            aria-label={t("reload")}
            onClick={() => void load("")}
            disabled={busy}
          >
            <RefreshCw size={17} />
          </button>
        </div>
      )}
      {submitted !== null && (
        <div className="search-results">
          {visible.map((message) => (
            <button
              className="saved-row"
              key={message.id}
              onClick={() => void choose(message)}
            >
              {kind === "favorites" ? (
                <Bookmark size={18} />
              ) : (
                <MessageCircle size={18} />
              )}
              <div>
                <strong>{text(message)}</strong>
                <small>{date(message.createdAt)}</small>
              </div>
              <ChevronRight size={16} />
            </button>
          ))}
          {!visible.length && !busy && (
            <div className="empty">
              <div className="empty-icon">
                {kind === "favorites" ? <Bookmark /> : <Search />}
              </div>
              <h2>
                {t(
                  kind === "favorites" && !filter
                    ? "emptyFavorites"
                    : "noResults",
                )}
              </h2>
              <button
                className="secondary"
                onClick={() => void load(submitted)}
              >
                <RefreshCw size={16} />
                {t("reload")}
              </button>
            </div>
          )}
          {more && (
            <button
              className="secondary load-more"
              disabled={busy}
              onClick={() => void load(submitted, true)}
            >
              {t(busy ? "working" : "loadMore")}
            </button>
          )}
        </div>
      )}
    </>
  );
}
