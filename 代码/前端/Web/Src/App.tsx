import { isRectInVisibleViewport, useVisibleViewport } from "./useVisibleViewport";
import { authProtocol, authSnapshot, authSnapshotCurrent, clearInitialProofs, expireInitial, finishActivatedInitial, hasInitialProof, hasAuthResponsibility, pendingInitial, prepareInitial, retainInitialProof, unconfirmedInitial, type InitialAuth } from "./AuthSession";
import PrivateMedia from "./PrivateMedia";
import AudioRecorderPanel from "./AudioRecorderPanel";
import AttachmentSendPanel from "./AttachmentSendPanel";
import { AttachmentSendActions, type AttachmentSendContext } from "./AttachmentSendActions";
import { acceptAttachmentMessage, linkAttachmentEnqueue, withAttachmentPublication } from "./AttachmentSendStore";
import { attachmentLabel } from "./AttachmentSendLabels";
import { runDurableSync } from "./SyncEngine";
import { adoptSelfDraft, hasDurableSyncResponsibility, loadPersonalState } from "./SyncStore";
import { SelfStateActions } from "./SelfStateActions";
import SelfStatePanel from "./SelfStatePanel";
import SelfDraftRecovery from "./SelfDraftRecovery";
import DataExportPanel from "./DataExportPanel";
import PasswordChangePanel from "./PasswordChangePanel";
import type { PasswordChangeContext } from "./PasswordChangeActions";
import AccountDeletionPanel from "./AccountDeletionPanel";
import type { AccountDeletionContext } from "./AccountDeletionActions";
import { accountDeletionLabel } from "./AccountDeletionLabels";
import GroupGovernancePanel from "./GroupGovernancePanel";
import GroupJoinPanel from "./GroupJoinPanel";
import MigrationEntry from "./MigrationPanel";
import ReceiptEntry from "./ReceiptPanel";
import type { GroupGovernanceContext } from "./GroupGovernanceActions";
import { groupLabel } from "./GroupGovernanceLabels";
import type { DataExportContext } from "./DataExportActions";
import { emptyPersonalState, personalStateRecord, type PersonalState } from "./SelfStateProtocol";
import { selfLabel } from "./SelfStateLabels";
import { materializeSyncSpace, refreshSyncForwardViews, validOriginalMessageResult } from "./SyncView";
import { SyncProtocolError } from "./SyncProtocol";
import { applyGlobalMessages } from "./GlobalMessages";
import { applyKnownGlobals, bindMessageNamespace, refreshMessageFacts, subscribeGlobalMessages } from "./MessageFacts";
import {
  attachmentPath,
  messagePayload,
  mergeMessageProjection,
  unavailableForward,
} from "./forward";
import {
  Fragment,
  useState,
  useEffect,
  useRef,
  useCallback,
  type FormEvent,
  type ReactNode,
} from "react";
import {
  MessageCircle,
  Users,
  Bookmark,
  Settings,
  Plus,
  Search,
  ArrowLeft,
  MoreHorizontal,
  Paperclip,
  Send,
  Smile,
  Reply,
  Pin,
  Check,
  CheckCheck,
  RefreshCw,
  WifiOff,
  ShieldCheck,
  LogOut,
  Sun,
  Moon,
  Monitor,
  ChevronRight,
  X,
  FileText,
  Download,
  Trash2,
  Copy,
  Edit3,
  CornerUpRight,
  Lock,
  Globe,
  UserRound,
  Server,
  Image as ImageIcon,
  Mic,
} from "lucide-react";
import {
  api,
  json,
  ApiError,
  apiBytes,
  clearCredentials,
  resetRequestContext,
  withAuthLifecycle,
} from "./api";
import AdminPanel from "./AdminPanel";
import ReportPanel, { type ReportTarget } from "./ReportPanel";
import GroupControls, { type GroupAction } from "./GroupControls";
import PeerControls from "./PeerControls";
import MessageLookup from "./MessageLookup";
import MentionPicker from "./MentionPicker";
import AnnouncementPanel from "./AnnouncementPanel";
import { demoSpace, demoUser, demoUsers } from "./demo";
import {
  loadSpace,
  saveSpace,
  saveView,
  queueMessage,
  acknowledgeOriginalOutbox,
  freezeConversation,
  freezeOutboxMessage,
  freezeNamespace,
  isolateAuthNamespace,
  prepareOutboxAttempt,
  saveIntent,
  loadIntent,
  clearIntent,
} from "./storage";
import { translate, languages } from "./i18n";
import {
  emptySpace,
  type Space,
  type User,
  type Message,
  type Conversation,
  type Session,
} from "./types";
type OriginalOutboxProof = { clientMsgId: string; conversationId: string; originalBody: unknown };
async function proveOriginalOutbox(
  candidates: readonly Message[],
  key: string,
  principalId: string,
  epoch: string,
  isCurrent: () => boolean,
): Promise<OriginalOutboxProof[]> {
  const proofs: OriginalOutboxProof[] = [];
  const pending = candidates.filter(row => row.sendAttempted && !row.frozen && row.senderId === principalId && (row.restoreEpoch ?? "1") === epoch);
  for (let offset = 0; offset < pending.length; offset += 4) {
    await Promise.all(pending.slice(offset, offset + 4).map(async original => {
      const body = messagePayload(original);
      try {
        const accepted = await api<Message>("/conversations/" + encodeURIComponent(original.conversationId) + "/message-results", json("POST", body));
        if (!isCurrent()) throw new SyncProtocolError("SYNC_CONTEXT_CHANGED");
        if (!validOriginalMessageResult(accepted, principalId, original.conversationId, original.clientMsgId)) return;
        proofs.push({ clientMsgId: original.clientMsgId, conversationId: original.conversationId, originalBody: body });
      } catch (error) {
        if (!isCurrent()) throw new SyncProtocolError("SYNC_CONTEXT_CHANGED");
        if (error instanceof ApiError && ["NOT_A_MEMBER", "HISTORY_NOT_ALLOWED"].includes(error.code)) {
          if (isCurrent()) await freezeOutboxMessage(key, original.clientMsgId, error.code);
          throw new SyncProtocolError("SYNC_PERMISSION_CHANGED");
        }
        if (error instanceof ApiError && ["RESTORE_EPOCH_CHANGED", "STORE_GENERATION_CHANGED"].includes(error.code)) throw error;
        if (error instanceof ApiError && [403, 404, 409].includes(error.status) && !error.code.startsWith("AUTH_")) return;
        if (error instanceof ApiError && error.status >= 500 && !error.code.startsWith("AUTH_")) return;
        if (error instanceof TypeError || error instanceof DOMException && ["TimeoutError", "NetworkError"].includes(error.name)) return;
        throw error;
      }
    }));
  }
  return proofs;
}

type AvatarIntent = {
  namespace: string;
  requestId: string;
  expectedVersion: string;
  restoreEpoch: string;
  file: File;
  name: string;
};
type AvatarResult = {
  operationId: string;
  status: string;
  current: boolean;
  user: User;
};
const settle = async (jobs: Promise<unknown>[]) => {
  const results = await Promise.allSettled(jobs);
  const failure = results.find((result) => result.status === "rejected");
  if (failure?.status === "rejected") throw failure.reason;
};
const uid = () => crypto.randomUUID();
const pref = <T,>(key: string, fallback: T): T => {
  try {
    return JSON.parse(localStorage.getItem(key) ?? "null") ?? fallback;
  } catch {
    return fallback;
  }
};
function Avatar({
  name,
  size = 44,
  group = false,
  src,
}: {
  name: string;
  size?: number;
  group?: boolean;
  src?: string;
}) {
  const [image, setImage] = useState("/Logo.svg");
  useEffect(() => {
    let active = true,
      objectURL = "";
    const controller = new AbortController();
    setImage("/Logo.svg");
    if (!src || !/^\/api\/avatars\/[A-Za-z0-9_-]+$/.test(src)) return;
    void apiBytes(src.substring(4), { signal: controller.signal })
      .then((blob) => {
        if (!active) return;
        objectURL = URL.createObjectURL(blob);
        setImage(objectURL);
      })
      .catch(() => {
        if (active) setImage("/Logo.svg");
      });
    return () => {
      active = false;
      controller.abort();
      if (objectURL) URL.revokeObjectURL(objectURL);
    };
  }, [src]);
  return (
    <div
      className={"avatar " + (group ? "group-avatar" : "")}
      style={{ width: size, height: size, fontSize: size * 0.35 }}
    >
      {group ? (
        <Users size={size * 0.43} />
      ) : src ? (
        <img
          src={image}
          alt={name}
          width={size}
          height={size}
          onError={() => setImage("/Logo.svg")}
        />
      ) : (
        Array.from(name || "J")
          .slice(0, 2)
          .join("")
      )}
    </div>
  );
}
function IconButton({
  label,
  children,
  onClick,
  active = false,
  disabled = false,
  current = false,
}: {
  label: string;
  children: ReactNode;
  onClick?: () => void;
  active?: boolean;
  disabled?: boolean;
  current?: boolean;
}) {
  return (
    <button
      type="button"
      className={"icon-button " + (active ? "active" : "")}
      title={label}
      aria-label={label}
      aria-current={current ? "page" : undefined}
      onClick={onClick}
      disabled={disabled}
    >
      {children}
    </button>
  );
}
function MessageImage({ src, alt }: { src: string; alt: string }) {
  const [resolved, setResolved] = useState("/Logo.svg");
  useEffect(() => {
    let active = true;
    setResolved("/Logo.svg");
    const image = new window.Image();
    image.onload = () => {
      if (active) setResolved(src);
    };
    image.src = src;
    return () => {
      active = false;
      image.onload = null;
    };
  }, [src]);
  return (
    <a href={src} target="_blank" rel="noopener noreferrer">
      <img
        className={
          "message-image " + (resolved === "/Logo.svg" ? "media-fallback" : "")
        }
        src={resolved}
        alt={alt}
        width="280"
        height="180"
        onError={() => setResolved("/Logo.svg")}
      />
    </a>
  );
}
export default function App() {
  useVisibleViewport();
  const [locale, setLocale] = useState(() => pref("jobsim.locale", "zh-CN"));
  const t = useCallback((k: string) => translate(locale, k), [locale]);
  const [theme, setTheme] = useState(() => pref("jobsim.theme", "system"));
  const [font, setFont] = useState(() => pref("jobsim.font", "normal"));
  const [personalHolder, setPersonalHolder] = useState<{ namespace: string; data: PersonalState }>({ namespace: "", data: emptyPersonalState() });
  const [chatFilter, setChatFilter] = useState("inbox");
  const [selfBusy, setSelfBusy] = useState(false);
  const [fontPending, setFontPending] = useState<string[]>([]);
  const [readCounts, setReadCounts] = useState<{ namespace: string; counts: Record<string, { count: string; watermark: string; localRevision?: string }> }>({ namespace: "", counts: {} });
  const [visibleRevision, setVisibleRevision] = useState(0);
  const [notify, setNotify] = useState(() =>
    pref("jobsim.notifications", false),
  );
  const [viewActive, setViewActive] = useState(
    () => document.visibilityState === "visible" && document.hasFocus(),
  );
  const readingBottom = useRef(true);
  const [demo, setDemo] = useState(true);
  const [live, setLiveState] = useState<Space>(emptySpace);
  const [sample, setSample] = useState<Space>(() => demoSpace());
  const [session, setSession] = useState<Session | null>(null);
  const [deployment, setDeployment] = useState("");
  const [online, setOnline] = useState(false);
  const [booted, setBooted] = useState(false);
  const [section, setSection] = useState("messages");
  const [selected, setSelected] = useState("demo-design");
  const [mobileOpen, setMobileOpen] = useState(false);
  const [query, setQuery] = useState("");
  const [modal, setModalState] = useState("");
  const modalGeneration = useRef(0);
  const modalValue = useRef("");
  const setModal = useCallback((value: string) => {
    modalGeneration.current++;
    modalValue.current = value;
    setModalState(value);
  }, []);
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [attachmentRevision, setAttachmentRevision] = useState(0);
  const [requiresOTP, setRequiresOTP] = useState(false);
  const [avatarPending, setAvatarPending] = useState<AvatarIntent | null>(null);
  const [reply, setReply] = useState<Message | null>(null);
  const [editing, setEditing] = useState<Message | null>(null);
  const [more, setMore] = useState("");
  const [details, setDetails] = useState(false);
  const [reportTarget, setReportTarget] = useState<ReportTarget | null>(null);
  const [announcementSource, setAnnouncementSource] = useState<Message | null>(
    null,
  );
  const [contacts, setContacts] = useState<User[]>(
    demoUsers.filter((u) => u.id !== demoUser.id),
  );
  const [requests, setRequests] = useState<any[]>([]);
  const [people, setPeople] = useState<User[]>([]);
  const [members, setMembers] = useState<User[]>([]);
  const [memberCursor, setMemberCursor] = useState("");
  const [generated, setGenerated] = useState<string[]>([]);
  const [sessions, setSessions] = useState<any[]>([]);
  const [pick, setPick] = useState<string[]>([]);
  const [forwarding, setForwarding] = useState<Message | null>(null);
  const [lookupReload, setLookupReload] = useState(0);
  const fileInput = useRef<HTMLInputElement>(null);
  const avatarInput = useRef<HTMLInputElement>(null);
  const capabilityChecked = useRef(0);
  const mentionReviewOffset = useRef(0);
  const forwardReviewOffset = useRef(0);
  const outboxReviewOffset = useRef(0);
  const composer = useRef<HTMLTextAreaElement>(null);
  const thread = useRef<HTMLDivElement>(null);
  const namespace = useRef("");
  const localDraftRevision = useRef(0);
  const composerMetadata = useRef({ reply, editing });
  composerMetadata.current = { reply, editing };
  const observationBusy = useRef(false);
  const observedLocally = useRef(new Set<string>());
  const globalMessages = useRef({ scope: "", ids: new Set<string>() });
  const globalStorageFailure = useRef(false);
  const forwardProjectionGeneration = useRef(0);
  const changing = useRef(0);
  const currentSelection = useRef(selected);
  currentSelection.current = selected;
  const grouping = useRef(false);
  const syncing = useRef(false);
  const sending = useRef(false);
  const authRequest = useRef("");
  const peopleRequest = useRef(0);
  const [peopleQueried, setPeopleQueried] = useState(false);
  const [features, setFeatures] = useState<Record<string, boolean>>({});
  const featureState = useRef<Record<string, boolean>>({});
  const syncGuard = useRef({ namespace: "", required: false, ready: false });
  const publishFeatures = (next: Record<string, boolean>) => {
    featureState.current = next;
    setFeatures(next);
  };
  const liveRef = useRef(live);
  liveRef.current = live;
  const setLive = useCallback((update: Space | ((previous: Space) => Space)) => {
    setLiveState(previous => {
      const next = typeof update === "function" ? update(previous) : update;
      const facts = globalMessages.current;
      const safe = namespace.current && facts.scope === namespace.current.split("|").slice(0, 4).join("|")
        ? applyKnownGlobals(next, facts.ids) : next;
      return namespace.current && globalStorageFailure.current ? { ...safe, conversations: [], messages: {}, cursors: {}, profiles: {}, unread: {}, mentionUnread: {} } : safe;
    });
  }, []);
  useEffect(() => subscribeGlobalMessages(notice => {
    if (!namespace.current || namespace.current.split("|").slice(0, 4).join("|") !== notice.scope) return;
    const previous = globalMessages.current;
    const changed = previous.scope !== notice.scope || notice.ids.some(id => !previous.ids.has(id));
    globalMessages.current = { scope: notice.scope, ids: new Set(notice.ids) };
    if (notice.storageFailed !== undefined) globalStorageFailure.current = notice.storageFailed;
    if (notice.storageFailed) {
      syncGuard.current.ready = false;
      setNotice(t("syncRestricted"));
    }
    if (!changed && !notice.storageFailed) return;
    forwardProjectionGeneration.current++;
    liveRef.current = applyGlobalMessages(liveRef.current, globalMessages.current.ids);
    setLive(liveRef.current);
    setReply(null);
    setEditing(null);
    setForwarding(null);
    setAnnouncementSource(null);
    setReportTarget(null);
    setMore("");
    setDetails(false);
    setLookupReload(value => value + 1);
    if (["forward", "announcement", "search", "reports"].includes(modalValue.current)) setModal("");
  }), [setLive, setModal, t]);
  const space = demo ? sample : live;
  const personal = !demo && personalHolder.namespace === namespace.current ? personalHolder.data : emptyPersonalState();
  const cloudFont = personalStateRecord(personal, "preferences")?.fontPreset;
  const effectiveFont = !demo && features.sync_self_state === true && typeof cloudFont === "string" ? cloudFont === "standard" ? "normal" : cloudFont : font;
  const me = demo ? demoUser : session?.user;
  const conversation = space.conversations.find((c) => c.id === selected);
  const readContext = useRef({ selected, section, active: viewActive });
  readContext.current = { selected, section, active: viewActive };
  const notifyRef = useRef(notify);
  notifyRef.current = notify;
  const draft = space.drafts[selected] ?? "";
  const mentionDraft = space.mentionDrafts?.[selected] ?? {
    userIds: [],
    all: false,
  };
  const messages = (space.messages[selected] ?? []).filter(
    (m) => section !== "favorites" || m.favorite,
  );
  const enabled = (name: string) => demo || features[name] === true;
  const canSend =
    (demo || !globalStorageFailure.current) &&
    !!conversation &&
    !conversation.readOnly &&
    (demo ||
      (features.send_admission === true && !!conversation.sendAdmission));
  const setSpace = (fn: (s: Space) => Space) => {
    if (demo) setSample(fn);
    else setLive(fn);
  };
  useEffect(() => {
    localStorage.setItem("jobsim.notifications", JSON.stringify(notify));
  }, [notify]);
  useEffect(() => {
    const update = () =>
      setViewActive(
        document.visibilityState === "visible" && document.hasFocus(),
      );
    document.addEventListener("visibilitychange", update);
    window.addEventListener("focus", update);
    window.addEventListener("blur", update);
    return () => {
      document.removeEventListener("visibilitychange", update);
      window.removeEventListener("focus", update);
      window.removeEventListener("blur", update);
    };
  }, []);
  useEffect(() => {
    const resize = () => setVisibleRevision(value => value + 1);
    window.addEventListener("resize", resize);
    return () => window.removeEventListener("resize", resize);
  }, []);
  const markRead = () => {
    if (!demo && featureState.current.sync_self_state === true) return;
    if (
      viewActive &&
      section === "messages" &&
      selected &&
      readingBottom.current
    )
      setSpace((value) =>
        value.unread?.[selected]?.length ||
        value.mentionUnread?.[selected]?.length
          ? {
              ...value,
              unread: { ...value.unread, [selected]: [] },
              mentionUnread: { ...value.mentionUnread, [selected]: [] },
            }
          : value,
      );
  };
  useEffect(() => {
    markRead();
  }, [selected, section, viewActive, demo]);

  const invalidateSession = () => {
    changing.current++;
    resetRequestContext();
    namespace.current = "";
    setSession(null);
    setLive(emptySpace());
    setSelected("");
    setReply(null);
    setEditing(null);
    setForwarding(null);
    setAnnouncementSource(null);
    setModal("signIn");
  };
  const report = (e: unknown) => {
    if ((e instanceof ApiError && e.code === "AUTH_REQUEST_ABORTED") || (e instanceof DOMException && e.name === "AbortError")) return;
    if (e instanceof SyncProtocolError && e.code === "SELF_LOCAL_DRAFT_CHANGED") { setNotice(selfLabel(locale, "localChanged")); return; }
    if (e instanceof SyncProtocolError && e.code === "SELF_STATE_PENDING_OPERATION") { setNotice(selfLabel(locale, "pendingState")); return; }
    if (e instanceof SyncProtocolError && e.code === "SELF_STATE_VERSION_CHANGED") { setNotice(selfLabel(locale, "stateChanged")); return; }
    if (e instanceof ApiError && ["PREFERENCE_VERSION_CONFLICT", "VIEW_VERSION_CONFLICT", "MANUAL_UNREAD_CHANGED", "DRAFT_VERSION_CONFLICT"].includes(e.code)) { setNotice(selfLabel(locale, "stateChanged")); return; }
    if (e instanceof ApiError && ["AUTH_REFRESH_UNKNOWN", "AUTH_CONTEXT_CHANGED"].includes(e.code)) {
      setNotice(t(e.code === "AUTH_REFRESH_UNKNOWN" ? "authRefreshUnknown" : "authContextChanged"));
      return;
    }
    if (e instanceof ApiError && e.status === 401 && session) {
      invalidateSession();
      setNotice(t("sessionExpired"));
      return;
    }
    setNotice(
      e instanceof ApiError && e.status === 401
        ? t(
            ["signIn", "register", "recovery"].includes(modal)
              ? "authFailed"
              : "sessionExpired",
          )
        : e instanceof ApiError && e.status === 404
          ? t("notSupported")
          : t("failure"),
    );
  };
  useEffect(() => {
    const required = (event: Event) => {
      const code = (event as CustomEvent<{ code: string }>).detail.code;
      invalidateSession();
      setNotice(t(code === "AUTH_REFRESH_UNKNOWN" ? "authRefreshUnknown" : code === "AUTH_CONTEXT_CHANGED" ? "authContextChanged" : "sessionExpired"));
    };
    window.addEventListener("jobsim-auth-required", required);
    return () => window.removeEventListener("jobsim-auth-required", required);
  }, [t]);
  useEffect(() => {
    document.documentElement.lang = locale;
    localStorage.setItem("jobsim.locale", JSON.stringify(locale));
  }, [locale]);
  useEffect(() => {
    localStorage.setItem("jobsim.theme", JSON.stringify(theme));
    localStorage.setItem("jobsim.font", JSON.stringify(font));
    const media = matchMedia("(prefers-color-scheme: dark)");
    const apply = () => {
      document.documentElement.dataset.theme =
        theme === "system" ? (media.matches ? "dark" : "light") : theme;
      document.documentElement.dataset.font = effectiveFont;
    };
    apply();
    media.addEventListener("change", apply);
    return () => media.removeEventListener("change", apply);
  }, [theme, font, effectiveFont]);
  useEffect(() => {
    authRequest.current = "";
    clearInitialProofs();
    setRequiresOTP(false);
    setPeopleQueried(false);
    if (!modal) return;
    const previous = document.activeElement as HTMLElement | null;
    const id = requestAnimationFrame(() => {
      const dialog = document.querySelector(".modal");
      if (dialog?.contains(document.activeElement)) return;
      (
        document.querySelector<HTMLElement>(".modal input") ??
        document.querySelector<HTMLElement>(".modal button")
      )?.focus();
    });
    return () => {
      cancelAnimationFrame(id);
      previous?.focus();
    };
  }, [modal]);
  useEffect(() => {
    if (!notice) return;
    const id = setTimeout(() => setNotice(""), 6000);
    return () => clearTimeout(id);
  }, [notice]);
  const activate = async (s: Session, dep: string) => {
    resetRequestContext();
    const generation = ++changing.current;
    const epoch = s.restoreEpoch ?? "1";
    const source = s.sourceContextId ?? "personal";
    const legacyKey = location.origin + "|" + dep + "|" + s.user.id + "|personal|" + s.deviceId;
    const key = authProtocol()
      ? location.origin + "|" + dep + "|" + s.user.id + "|" + source + "|" + epoch + "|" + s.deviceId
      : legacyKey;
    if (key !== legacyKey) {
      await isolateAuthNamespace(location.origin + "|" + dep + "|" + s.user.id + "|" + source, key);
    }
    let stored = await loadSpace(key);
    if ((stored.restoreEpoch ?? "1") !== epoch) {
      await saveSpace(key + "|archive|" + (stored.restoreEpoch ?? "1"), stored);
      stored = await freezeNamespace(
        key,
        {
          ...stored,
          restoreEpoch: epoch,
          conversations: [],
          messages: {},
          cursors: {},
        },
        stored.restoreEpoch ?? "1",
      );
      setNotice(t("restoreChanged"));
    }
    stored = { ...stored, restoreEpoch: epoch };
    const requiresSync = featureState.current.authoritative_sync === true || await hasDurableSyncResponsibility(key);
    const globals = await refreshMessageFacts(key, () => generation === changing.current);
    if (generation !== changing.current) return;
    syncGuard.current = { namespace: key, required: requiresSync, ready: false };
    namespace.current = key;
    globalStorageFailure.current = false;
    bindMessageNamespace(key);
    globalMessages.current = { scope: key.split("|").slice(0, 4).join("|"), ids: globals };
    forwardProjectionGeneration.current++;
    setLive(requiresSync ? { ...stored, conversations: [], messages: {}, cursors: {}, profiles: {}, unread: {}, mentionUnread: {} } : stored);
    setSession(s);
    setContacts([]);
    setRequests([]);
    setDemo(false);
    setSelected(requiresSync ? "" : stored.conversations[0]?.id ?? "");
    setSection("messages");
    setMobileOpen(false);
    if (requiresSync) setNotice(t("syncPreparing"));
    finishActivatedInitial(s.familyId);
  };
  const bootstrap = useCallback(() => withAuthLifecycle(async () => {
    try {
      const b = await api<{
        deploymentId: string;
        name: string;
        features: Record<string, boolean>;
        restoreEpoch: string;
      }>("/bootstrap", {}, 3000);
      setDeployment(b.deploymentId);
      publishFeatures(b.features ?? {});
      setOnline(true);
      try {
        if (authProtocol() && unconfirmedInitial())
          await api("/auth/confirm", json("POST", {}));
        const m = await api<Session>("/auth/context");
        await activate(
          { ...m, restoreEpoch: m.restoreEpoch ?? b.restoreEpoch },
          b.deploymentId,
        );
      } catch (e) {
        if (!(e instanceof ApiError && e.status === 401))
          setNotice(t("connectionError"));
      }
    } catch {
      setOnline(false);
    } finally {
      setBooted(true);
    }
  }).catch(report), [t]);
  useEffect(() => {
    void bootstrap();
  }, []);
  useEffect(() => {
    const key = (e: KeyboardEvent) => {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === "k") {
        e.preventDefault();
        const search = document.querySelector<HTMLInputElement>(
          ".sidebar .search input",
        );
        if (search && search.getClientRects().length && getComputedStyle(search).visibility !== "hidden") search.focus();
        else setModal("search");
      }
      if (e.key === "Escape") setMore("");
    };
    window.addEventListener("keydown", key);
    return () => window.removeEventListener("keydown", key);
  }, []);
  useEffect(() => {
    if (!namespace.current || !session) return;
    if (syncGuard.current.required && !syncGuard.current.ready) return;
    void saveView(namespace.current, live).catch(() => setNotice(t("failure")));
  }, [live, session, t]);
  useEffect(() => {
    const guard = (e: BeforeUnloadEvent) => {
      if (Object.values(live.drafts).some(Boolean) || live.outbox.length) {
        e.preventDefault();
        e.returnValue = "";
      }
    };
    window.addEventListener("beforeunload", guard);
    return () => window.removeEventListener("beforeunload", guard);
  }, [live]);
  const sync = useCallback(async () => {
    if (demo || !session || syncing.current) return;
    syncing.current = true;
    const current = changing.current;
    try {
      if (Date.now() - capabilityChecked.current > 30000) {
        const capabilities = await api<{ features: Record<string, boolean> }>(
          "/capabilities",
        );
        if (current !== changing.current) return;
        publishFeatures(capabilities.features);
        capabilityChecked.current = Date.now();
      }
      const key = namespace.current;
      if (featureState.current.authoritative_sync === true) syncGuard.current = { ...syncGuard.current, namespace: key, required: true };
      if (syncGuard.current.namespace === key && syncGuard.current.required) {
        const isCurrent = () => current === changing.current && namespace.current === key;
        const restrict = () => {
          if (!isCurrent()) return;
          syncGuard.current.ready = false;
          setLive(value => ({ ...value, conversations: [], messages: {}, cursors: {}, profiles: {}, unread: {}, mentionUnread: {} }));
          setSelected("");
          setReply(null);
          setEditing(null);
          setForwarding(null);
          setAnnouncementSource(null);
          setReportTarget(null);
          setMembers([]);
          setMore("");
          if (!["signIn", "register", "recovery", "connect", ""].includes(modalValue.current)) setModal("");
        };
        if (featureState.current.authoritative_sync !== true) {
          restrict();
          throw new SyncProtocolError("SYNC_CAPABILITY_REQUIRED");
        }
        const wasReady = syncGuard.current.ready;
        const result = await runDurableSync({
          namespace: key,
          binding: { scopeId: deployment, principalId: session.user.id, sourceContextId: "personal", deviceId: session.deviceId, restoreEpoch: session.restoreEpoch ?? "1" },
          request: (path, init) => api(path, init, 15000),
          isCurrent,
          materialize: (previous, projections, mode) => materializeSyncSpace(previous, projections, {
            ...mode,
            currentUserId: session.user.id,
            readingConversationId: readContext.current.active && readContext.current.section === "messages" && readingBottom.current ? readContext.current.selected : undefined,
          }),
          reconcileOutbox: async (candidates: readonly Message[]) => {
            const pending = candidates.filter(row => row.sendAttempted && !row.frozen && row.senderId === session.user.id && (row.restoreEpoch ?? "1") === (session.restoreEpoch ?? "1"));
            if (!pending.length) return [];
            const start = outboxReviewOffset.current % pending.length;
            const batch = [...pending.slice(start), ...pending.slice(0, start)].slice(0, 32);
            outboxReviewOffset.current = start + batch.length;
            return proveOriginalOutbox(batch, key, session.user.id, session.restoreEpoch ?? "1", isCurrent);
          },
          onRestriction: restrict,
        });
        if (!isCurrent()) return;
        const personalState = featureState.current.sync_self_state === true
          ? await loadPersonalState(key, { scopeId: deployment, principalId: session.user.id, sourceContextId: "personal", deviceId: session.deviceId, restoreEpoch: session.restoreEpoch ?? "1" }, isCurrent)
          : emptyPersonalState();
        if (!isCurrent()) return;
        setPersonalHolder({ namespace: key, data: personalState });
        const globals = await refreshMessageFacts(key, isCurrent);
        if (!isCurrent()) return;
        globalMessages.current = { scope: key.split("|").slice(0, 4).join("|"), ids: globals };
        result.space = applyKnownGlobals(result.space, globals);
        const forwardGeneration = forwardProjectionGeneration.current;
        const candidates = Object.values(result.space.messages).flat().filter(row => row.forwarded && !row.pending && !row.deleted && !row.contentUnavailable);
        candidates.sort((a, b) => Number(b.conversationId === selected) - Number(a.conversationId === selected) || b.createdAt.localeCompare(a.createdAt));
        let refreshed = await refreshSyncForwardViews(result.space, candidates.slice(0, 16), api, isCurrent);
        if (!isCurrent()) return;
        if (forwardGeneration !== forwardProjectionGeneration.current) refreshed = applyGlobalMessages(refreshed, globalMessages.current.ids);
        if (wasReady && syncGuard.current.ready && document.visibilityState === "hidden" && notifyRef.current && "Notification" in window && Notification.permission === "granted") {
          const known = new Set(Object.values(liveRef.current.messages).flat().map(row => row.id));
          for (const conversation of refreshed.conversations) {
            const muted = personalStateRecord(personalState, "conversation_view", conversation.id)?.mutedUntil;
            if (typeof muted === "string" && Date.parse(muted) > Date.now()) continue;
            if (!(refreshed.messages[conversation.id] ?? []).some(row => !row.pending && !row.deleted && !row.contentUnavailable && row.senderId !== session.user.id && !known.has(row.id))) continue;
            try {
              const notification = new Notification("JobsIM", { body: t("messages"), tag: conversation.id });
              notification.onclick = () => { notification.close(); if (isCurrent()) { window.focus(); openConversation(conversation); } };
            } catch { /* Notifications run only after durable synchronization and must not block it. */ }
          }
        }
        syncGuard.current.ready = true;
        setNotice(value => value === t("syncPreparing") || value === t("syncRestricted") ? "" : value);
        setLive(refreshed);
        const visible = new Map(Object.values(refreshed.messages).flat().map(row => [row.id, row]));
        const currentMessage = (row: Message | null) => {
          const fresh = row ? visible.get(row.id) : undefined;
          return fresh && !fresh.deleted && !fresh.contentUnavailable && !fresh.forwardUnavailable ? fresh : null;
        };
        setReply(currentMessage);
        setEditing(currentMessage);
        setForwarding(currentMessage);
        setAnnouncementSource(currentMessage);
        setReportTarget(target => target?.targetType === "message" && !currentMessage(visible.get(target.targetId) ?? null) ? null : target);
        if (!selected || !result.space.conversations.some(row => row.id === selected)) setSelected(result.space.conversations[0]?.id ?? "");
        setOnline(true);
        return;
      }
      const cs = await api<{ items: Conversation[] }>("/conversations");
      if (current !== changing.current) return;
      const conversations = cs.items ?? [];
      if (selected && !conversations.some((item) => item.id === selected)) {
        try {
          const currentConversation = await api<Conversation>(
            "/conversations/" + encodeURIComponent(selected),
          );
          if (current !== changing.current) return;
          conversations.push(currentConversation);
        } catch (error) {
          if (!(error instanceof ApiError && [403, 404].includes(error.status)))
            throw error;
        }
      }
      const allowed = new Set(conversations.map((c) => c.id));
      setLive((s) => ({
        ...s,
        conversations,
        messages: Object.fromEntries(
          Object.entries(s.messages).filter(([id]) => allowed.has(id)),
        ),
      }));
      if (!selected || !allowed.has(selected))
        setSelected(conversations[0]?.id ?? "");
      const ordered = [...conversations].sort((a, b) =>
        a.id === selected ? -1 : b.id === selected ? 1 : 0,
      );
      for (let offset = 0; offset < ordered.length; offset += 4) {
        await settle(
          ordered.slice(offset, offset + 4).map(async (conversation) => {
            const peerId = conversation.memberIds.find(
              (id) => id !== session.user.id,
            );
            if (
              conversation.id === selected ||
              (!conversation.title &&
                peerId &&
                !liveRef.current.profiles?.[peerId])
            ) {
              const roster = await api<{ items: User[] }>(
                "/conversations/" +
                  encodeURIComponent(conversation.id) +
                  "/members",
              );
              if (current !== changing.current) return;
              setLive((s) => ({
                ...s,
                profiles: {
                  ...s.profiles,
                  ...Object.fromEntries(
                    (roster.items ?? []).map((user) => [user.id, user]),
                  ),
                },
              }));
            }

            let cursor = liveRef.current.cursors[conversation.id] ?? "0";
            for (let page = 0; page < 5; page++) {
              if (current !== changing.current) return;
              const r = await api<{ items: Message[]; cursor: string }>(
                "/conversations/" +
                  encodeURIComponent(conversation.id) +
                  "/messages?limit=200&after=" +
                  encodeURIComponent(cursor),
              );
              if (current !== changing.current) return;
              const previous = cursor;
              const incoming = r.items ?? [];
              const key = namespace.current;
              const isCurrent = () => current === changing.current && key === namespace.current;
              const candidateSpace = await loadSpace(key);
              const candidateIDs = new Set(incoming.filter(row => row.senderId === session.user.id).map(row => row.clientMsgId));
              const proofs = await proveOriginalOutbox(candidateSpace.outbox.filter(row => candidateIDs.has(row.clientMsgId)), key, session.user.id, session.restoreEpoch ?? "1", isCurrent);
              const acknowledged = await acknowledgeOriginalOutbox(key, session.user.id, proofs, isCurrent);
              if (current !== changing.current) return;
              const hadHistory = previous.split(":").at(-1) !== "0";
              if (
                hadHistory &&
                document.visibilityState === "hidden" &&
                "Notification" in window &&
                Notification.permission === "granted" &&
                notifyRef.current &&
                incoming.some(
                  (m) =>
                    m.senderId !== session.user.id &&
                    !m.deleted &&
                    !liveRef.current.messages[conversation.id]?.some(
                      (old) => old.id === m.id,
                    ),
                )
              ) {
                try {
                  const notification = new Notification("JobsIM", {
                    body: t("messages"),
                    tag: conversation.id,
                  });
                  notification.onclick = () => {
                    notification.close();
                    if (current === changing.current) {
                      window.focus();
                      openConversation(conversation);
                    }
                  };
                } catch {
                  /* Notification availability must not interrupt message persistence. */
                }
              }
              setLive((s) => {
                const accepted = new Set(acknowledged);
                const map = new Map(
                  (s.messages[conversation.id] ?? [])
                    .filter((m) => !m.pending || !accepted.has(m.clientMsgId))
                    .map((m) => [m.id, m]),
                );
                const reading =
                  readContext.current.active &&
                  readContext.current.section === "messages" &&
                  readContext.current.selected === conversation.id &&
                  readingBottom.current;
                const unread = new Set(
                  reading ? [] : (s.unread?.[conversation.id] ?? []),
                );
                const mentionUnread = new Set(
                  reading ? [] : (s.mentionUnread?.[conversation.id] ?? []),
                );
                for (const m of incoming) {
                  const previous = map.get(m.id);
                  if (m.deleted) {
                    unread.delete(m.id);
                    mentionUnread.delete(m.id);
                  } else if (
                    !previous &&
                    !reading &&
                    m.senderId !== session.user.id
                  ) {
                    unread.add(m.id);
                    if (m.mentionedMe) mentionUnread.add(m.id);
                  }
                  if (!m.mentionedMe) mentionUnread.delete(m.id);
                  if (previous && BigInt(previous.version) > BigInt(m.version))
                    continue;
                  map.set(
                    m.id,
                    mergeMessageProjection(previous, {
                      ...m,
                      reactions: m.reactions ?? [],
                    }),
                  );
                }
                return {
                  ...s,
                  unread: { ...s.unread, [conversation.id]: [...unread] },
                  mentionUnread: {
                    ...s.mentionUnread,
                    [conversation.id]: [...mentionUnread],
                  },
                  outbox: s.outbox.filter((m) => !accepted.has(m.clientMsgId)),
                  messages: {
                    ...s.messages,
                    [conversation.id]: [...map.values()].sort(
                      (a, b) =>
                        Date.parse(a.createdAt) - Date.parse(b.createdAt) ||
                        a.id.localeCompare(b.id),
                    ),
                  },
                  cursors: { ...s.cursors, [conversation.id]: r.cursor },
                };
              });
              cursor = r.cursor;
              if (cursor.split(":").at(-1) === previous.split(":").at(-1))
                break;
            }
          }),
        );
      }
      const unreadMentions = Object.entries(
        liveRef.current.mentionUnread ?? {},
      ).flatMap(([conversationId, ids]) =>
        ids.map((id) => ({ conversationId, id })),
      );
      if (unreadMentions.length) {
        const start = mentionReviewOffset.current % unreadMentions.length;
        const batch = [
          ...unreadMentions.slice(start),
          ...unreadMentions.slice(0, start),
        ].slice(0, 20);
        mentionReviewOffset.current = start + batch.length;
        await settle(
          batch.map(async ({ conversationId, id }) => {
            let message: Message | undefined;
            try {
              message = await api<Message>(
                "/messages/" + encodeURIComponent(id),
              );
            } catch (error) {
              if (
                !(
                  error instanceof ApiError && [403, 404].includes(error.status)
                )
              )
                throw error;
            }
            if (current !== changing.current) return;
            const projection = message;
            setLive((value) => ({
              ...value,
              mentionUnread: {
                ...value.mentionUnread,
                [conversationId]:
                  projection?.mentionedMe && !projection.deleted
                    ? (value.mentionUnread?.[conversationId] ?? [])
                    : (value.mentionUnread?.[conversationId] ?? []).filter(
                        (item) => item !== id,
                      ),
              },
              messages: {
                ...value.messages,
                [conversationId]: (
                  value.messages[conversationId] ?? []
                ).flatMap((old) =>
                  old.id !== id
                    ? [old]
                    : !projection
                      ? []
                      : BigInt(old.version) > BigInt(projection.version)
                        ? [old]
                        : [mergeMessageProjection(old, projection)],
                ),
              },
            }));
          }),
        );
      }
      const forwards = Object.values(liveRef.current.messages)
        .flat()
        .filter(
          (message) =>
            message.forwarded && !message.pending && !message.deleted,
        );
      if (forwards.length) {
        const start = forwardReviewOffset.current % forwards.length;
        const batch = [
          ...forwards.slice(start),
          ...forwards.slice(0, start),
        ].slice(0, 20);
        forwardReviewOffset.current = start + batch.length;
        await settle(
          batch.map(async (cached) => {
            const forwardGeneration = forwardProjectionGeneration.current;
            let projection: Message;
            try {
              projection = await api<Message>(
                "/messages/" + encodeURIComponent(cached.id),
              );
            } catch (error) {
              if (
                error instanceof ApiError &&
                (error.status === 401 || error.code === "RESTORE_EPOCH_CHANGED")
              )
                throw error;
              projection = unavailableForward(cached);
            }
            if (current !== changing.current) return;
            if (forwardGeneration !== forwardProjectionGeneration.current) projection = unavailableForward(projection);
            setLive((value) => ({
              ...value,
              messages: {
                ...value.messages,
                [cached.conversationId]: (
                  value.messages[cached.conversationId] ?? []
                ).map((old) =>
                  old.id !== cached.id ||
                  BigInt(old.version) > BigInt(projection.version)
                    ? old
                    : projection,
                ),
              },
            }));
          }),
        );
      }
      setOnline(true);
    } catch (e) {
      if (current !== changing.current) return;
      setOnline(false);
      setLive((value) => ({
        ...value,
        messages: Object.fromEntries(
          Object.entries(value.messages).map(([id, rows]) => [
            id,
            rows.map((message) =>
              message.forwarded && !message.pending
                ? unavailableForward(message)
                : message,
            ),
          ]),
        ),
      }));
      if (e instanceof ApiError && e.code === "RESTORE_EPOCH_CHANGED") {
        changing.current++;
        resetRequestContext();
        setNotice(t("restoreChanged"));
        setSession(null);
        setModal("signIn");
        namespace.current = "";
      } else if (e instanceof ApiError && e.status === 401) {
        invalidateSession();
        setNotice(t("sessionExpired"));
      } else if (e instanceof ApiError && e.code === "STORE_GENERATION_CHANGED") {
        invalidateSession();
        setNotice(t("syncStoreChanged"));
      } else if (e instanceof SyncProtocolError) {
        setNotice(t("syncRestricted"));
      }
    } finally {
      syncing.current = false;
    }
  }, [demo, session, selected, deployment, features.authoritative_sync, t]);
  useEffect(() => {
    void sync();
    const id = setInterval(() => {
      void sync();
    }, 3000);
    return () => clearInterval(id);
  }, [sync]);
  const refreshContacts = useCallback(async () => {
    if (demo || !session) return;
    const generation = changing.current;
    try {
      const result = await api<{ items: User[]; requests: any[] }>("/contacts");
      if (generation !== changing.current) return;
      setContacts(result.items ?? []);
      setRequests(
        (result.requests ?? []).filter(
          (request) =>
            request.state === "pending" &&
            request.recipientId === session.user.id,
        ),
      );
    } catch (error) {
      if (generation === changing.current) report(error);
    }
  }, [demo, session]);
  useEffect(() => {
    void refreshContacts();
    if (section !== "contacts") return;
    const timer = setInterval(() => void refreshContacts(), 5000);
    return () => clearInterval(timer);
  }, [section, refreshContacts]);
  const resolveRequest = async (requestId: string, accept: boolean) => {
    const generation = changing.current;
    try {
      await api(
        "/contacts/requests/" +
          encodeURIComponent(requestId) +
          "/" +
          (accept ? "accept" : "reject"),
        json("POST", {}),
      );
      if (generation !== changing.current) return;
      setNotice(t("saved"));
      await refreshContacts();
    } catch (error) {
      if (generation === changing.current) report(error);
    }
  };
  useEffect(() => {
    setReply(null);
    setEditing(null);
    setMore("");
    setDetails(false);
    setMembers([]);
    setMemberCursor("");
  }, [selected]);
  useEffect(() => {
    const box = thread.current;
    if (box && readingBottom.current) box.scrollTop = box.scrollHeight;
  }, [messages.length]);
  const updateDraft = (value: string) => {
    localDraftRevision.current++;
    setSpace((s) => ({ ...s, drafts: { ...s.drafts, [selected]: value } }));
  };
  const captureSelfActions = () => {
    if (demo || !session || !namespace.current || !syncGuard.current.ready || features.sync_self_state !== true) throw new SyncProtocolError("SYNC_SELF_STATE_UNAVAILABLE");
    const auth = authSnapshot().auth;
    if (!auth || auth.userId !== session.user.id || auth.deviceId !== session.deviceId || auth.deploymentId !== deployment || auth.restoreEpoch !== (session.restoreEpoch ?? "1")) throw new SyncProtocolError("SYNC_CONTEXT_CHANGED");
    const key = namespace.current, generation = changing.current, family = auth.familyId;
    const isCurrent = () => namespace.current === key && changing.current === generation && authSnapshot().auth?.familyId === family;
    return new SelfStateActions({ namespace: key,
      identity: { scopeId: deployment, principalId: session.user.id, sourceContextId: "personal", deviceId: session.deviceId, restoreEpoch: session.restoreEpoch ?? "1" },
      isCurrent, request: (path, options) => api(path, options, 15000),
    });
  };
  const captureDataExportContext = (): DataExportContext | undefined => {
    if (demo || !session || !namespace.current) return undefined;
    const auth = authSnapshot().auth;
    if (!auth || auth.userId !== session.user.id || auth.deviceId !== session.deviceId || auth.deploymentId !== deployment || auth.restoreEpoch !== (session.restoreEpoch ?? "1")) return undefined;
    const key = namespace.current, generation = changing.current, family = auth.familyId;
    return { namespace: key,
      identity: { scopeId: deployment, userId: session.user.id, deviceId: session.deviceId, epoch: session.restoreEpoch ?? "1" },
      isCurrent: () => namespace.current === key && changing.current === generation && authSnapshot().auth?.familyId === family,
    };
  };
  const capturePasswordChangeContext = (): PasswordChangeContext | undefined => {
    if (!deployment) return undefined;
    const scopeId = deployment, generation = changing.current, family = authSnapshot().auth?.familyId;
    const original = captureDataExportContext();
    return { scopeId,
      identity: original && session ? { ...original.identity, username: session.user.username, epoch: original.identity.epoch, familyId: family!, namespace: original.namespace } : undefined,
      isCurrent: () => deployment === scopeId && changing.current === generation && authSnapshot().auth?.familyId === family,
      onPause: () => { invalidateSession(); },
    };
  };
  const captureAccountDeletionContext = (): AccountDeletionContext | undefined => {
    const context = capturePasswordChangeContext();
    return context ? { ...context, onPause: () => invalidateSession() } : undefined;
  };
  const captureGroupContext = (): GroupGovernanceContext | undefined => {
    const context = capturePasswordChangeContext();
    if (demo || !context?.identity) return undefined;
    return { identity: context.identity, isCurrent: context.isCurrent, request: (path, options) => api(path, options, 15000) };
  };
  useEffect(() => {
    setSelfBusy(false);
    setFontPending([]);
    setReadCounts({ namespace: "", counts: {} });
  }, [demo, session?.deviceId, session?.user.id, session?.restoreEpoch, deployment]);
  const changeFont = async (value: string) => {
    if (demo || features.sync_self_state !== true) { setFont(value); return; }
    if (selfBusy) return;
    const actions = captureSelfActions();
    setSelfBusy(true);
    try {
      const result = await actions.font(value === "normal" ? "standard" : value as "small" | "large");
      if (!actions.context.isCurrent()) return;
      setNotice(result.current ? t("saved") : selfLabel(locale, "oldResult"));
      await sync();
    } catch (error) { if (actions.context.isCurrent()) report(error); }
    finally {
      if (actions.context.isCurrent()) {
        try { setFontPending((await actions.operations()).filter(row => row.kind === "preferences" && ["pending", "prepared"].includes(row.status)).map(row => row.requestId)); }
        catch (error) { report(error); }
        setSelfBusy(false);
      }
    }
  };
  const retryFont = async () => {
    if (selfBusy) return;
    const actions = captureSelfActions();
    setSelfBusy(true);
    try {
      const pending = (await actions.operations()).filter(row => row.kind === "preferences" && ["pending", "prepared"].includes(row.status));
      for (const row of pending) await actions.resume(row.requestId, true);
      if (actions.context.isCurrent()) { setFontPending([]); await sync(); }
    } catch (error) { if (actions.context.isCurrent()) report(error); }
    finally { if (actions.context.isCurrent()) setSelfBusy(false); }
  };
  const adoptCloudText = async (text: string, expectedLocal: string) => {
    const actions = captureSelfActions(), conversationId = currentSelection.current, revision = localDraftRevision.current;
    const expectedMentions = structuredClone(liveRef.current.mentionDrafts?.[conversationId] ?? { userIds: [], all: false });
    const metadata = JSON.stringify(composerMetadata.current);
    const isCurrent = () => actions.context.isCurrent() && currentSelection.current === conversationId && revision === localDraftRevision.current && JSON.stringify(composerMetadata.current) === metadata && JSON.stringify(liveRef.current.mentionDrafts?.[conversationId] ?? { userIds: [], all: false }) === JSON.stringify(expectedMentions) && (liveRef.current.drafts[conversationId] ?? "") === expectedLocal;
    try {
      await adoptSelfDraft(actions.context.namespace, actions.context.identity, conversationId, expectedLocal, text, isCurrent, expectedMentions);
      if (!isCurrent()) throw new SyncProtocolError("SELF_LOCAL_DRAFT_CHANGED");
      localDraftRevision.current++;
      setLive(value => ({ ...value, drafts: { ...value.drafts, [conversationId]: text }, mentionDrafts: { ...value.mentionDrafts, [conversationId]: { userIds: [], all: false } } }));
      setReply(null);
      setEditing(null);
    } catch (error) {
      if (actions.context.isCurrent()) setNotice(selfLabel(locale, "localChanged"));
      throw error;
    }
  };
  useEffect(() => {
    if (demo || !session || features.sync_self_state !== true || !syncGuard.current.ready) return;
    const actions = captureSelfActions();
    void actions.operations().then(rows => {
      if (actions.context.isCurrent()) setFontPending(rows.filter(row => row.kind === "preferences" && ["pending", "prepared"].includes(row.status)).map(row => row.requestId));
    }).catch(() => {});
  }, [demo, session, features.sync_self_state, live.syncRevision]);
  useEffect(() => {
    if (demo || !session || !viewActive || modal || section !== "messages" || !selected || !conversation?.membershipGeneration || features.sync_self_state !== true || !syncGuard.current.ready) return;
    const original = captureSelfActions(), conversationId = selected, membership = conversation.membershipGeneration;
    let alive = true;
    const observing = () => alive && original.context.isCurrent() && !modalValue.current && document.visibilityState === "visible" && document.hasFocus() && readContext.current.active && readContext.current.section === "messages" && readContext.current.selected === conversationId && liveRef.current.conversations.find(row => row.id === conversationId)?.membershipGeneration === membership;
    const actions = new SelfStateActions({ ...original.context, isCurrent: observing });
    const timer = setTimeout(async () => {
      if (!observing() || observationBusy.current || !thread.current) return;
      const box = thread.current.getBoundingClientRect();
      const observed = messages.filter(row => !row.pending && !row.deleted && !row.contentUnavailable && row.senderId !== session.user.id).filter(row => {
        const article = document.getElementById("message-" + row.id), bubble = article?.querySelector(".bubble") ?? article;
        const rect = bubble?.getBoundingClientRect();
        if (!article || !rect || !isRectInVisibleViewport(rect, box)) return false;
        const x = (rect.left + rect.right) / 2, y = (rect.top + rect.bottom) / 2;
        return [[x, y], [rect.left + 2, y], [rect.right - 2, y], [x, rect.top + 2], [x, rect.bottom - 2]].every(([left, top]) => {
          const hit = document.elementFromPoint(left, top);
          return !!hit && article.contains(hit);
        });
      }).map(row => row.id).filter(id => !observedLocally.current.has(actions.context.namespace + "|" + conversation?.membershipGeneration + "|" + id)).slice(0, 200);
      observationBusy.current = true;
      try {
        if (observed.length) {
          const result = await actions.observe(conversationId, observed, membership);
          if (!alive || !actions.context.isCurrent()) return;
          if (result.status === "applied" && result.current) {
            for (const id of observed) observedLocally.current.add(actions.context.namespace + "|" + conversation?.membershipGeneration + "|" + id);
            const ids = new Set(observed);
            setLive(value => value.conversations.find(row => row.id === conversationId)?.membershipGeneration !== result.result.membershipGeneration ? value : ({ ...value,
              unread: { ...value.unread, [conversationId]: (value.unread?.[conversationId] ?? []).filter(id => !ids.has(id)) },
              mentionUnread: { ...value.mentionUnread, [conversationId]: (value.mentionUnread?.[conversationId] ?? []).filter(id => !ids.has(id)) },
            }));
          }
        }
        const count = await actions.get("read", conversationId);
        if (alive && actions.context.isCurrent()) setReadCounts(previous => ({ namespace: actions.context.namespace, counts: { ...(previous.namespace === actions.context.namespace ? previous.counts : {}), [conversationId]: { count: count.unreadCount!, watermark: count.countSourceWatermark!, localRevision: live.syncRevision } } }));
      } catch {
        if (alive && actions.context.isCurrent()) setReadCounts(previous => ({ namespace: actions.context.namespace, counts: Object.fromEntries(Object.entries(previous.namespace === actions.context.namespace ? previous.counts : {}).filter(([id]) => id !== conversationId)) }));
      } finally { observationBusy.current = false; }
    }, 300);
    return () => { alive = false; clearTimeout(timer); };
  }, [demo, session, viewActive, modal, details, mobileOpen, section, selected, features.sync_self_state, conversation?.membershipGeneration, messages.length, live.syncRevision, visibleRevision]);
  const writeMessage = async (m: Message, generation = changing.current) => {
    const originalFamily = m.originFamilyId;
    const sendCurrent = () => generation === changing.current && (!originalFamily || authSnapshot().auth?.familyId === originalFamily);
    if (!sendCurrent() || !session || demo || m.frozen) return;
    if (globalStorageFailure.current) { setNotice(t("syncRestricted")); return; }
    if (syncGuard.current.required && !syncGuard.current.ready) {
      setNotice(t("syncPreparing"));
      return;
    }
    if ((m.restoreEpoch ?? "1") !== (session.restoreEpoch ?? "1")) {
      setNotice(t("restoreChanged"));
      return;
    }
    try {
      const support = await api<{
        deploymentId: string;
        features: Record<string, boolean>;
      }>("/bootstrap");
      if (!sendCurrent()) return;
      if (support.deploymentId !== deployment) {
        setNotice(t("notSupported"));
        return;
      }
      const key = namespace.current;
      const original = await prepareOutboxAttempt(key, m.clientMsgId);
      if (
        !original ||
        !sendCurrent() ||
        key !== namespace.current
      )
        return;
      m = original;
      setLive((value) => ({
        ...value,
        outbox: value.outbox.map((message) =>
          message.clientMsgId === m.clientMsgId
            ? { ...message, sendAttempted: true }
            : message,
        ),
      }));
      const payload = messagePayload(m);
      let result: Message | undefined;
      if (m.sendAttempted) {
        try {
          result = await api<Message>(
            `/conversations/${m.conversationId}/message-results`,
            json("POST", payload),
          );
        } catch (error) {
          if (!(error instanceof ApiError && error.status === 404)) throw error;
        }
      }
      if (!sendCurrent()) return;
      if (!result) {
        if (
          support.features?.send_admission !== true ||
          (m.mentionAll && support.features?.role_admission !== true) ||
          (m.type === "forward" && support.features?.forwarding !== true)
        ) {
          setNotice(t("notSupported"));
          return;
        }
        const publish = () => api<Message>(`/conversations/${m.conversationId}/messages`, json("POST", payload));
        if (m.attachmentRequestId) {
          const context = captureAttachmentContext();
          if (!context || context.identity.familyId !== m.originFamilyId) return;
          result = await withAttachmentPublication(context, m, publish);
        } else result = await publish();
      }
      if (!sendCurrent()) return;
      if (!validOriginalMessageResult(result, session.user.id, m.conversationId, m.clientMsgId)) throw new SyncProtocolError("SYNC_INVALID_RECONCILIATION");
      const isCurrent = () => sendCurrent() && key === namespace.current;
      if (m.attachmentRequestId) {
        const context = captureAttachmentContext();
        if (!context || context.identity.familyId !== m.originFamilyId) return;
        await acceptAttachmentMessage(context, result);
        setAttachmentRevision(value => value + 1);
      }
      const removed = await acknowledgeOriginalOutbox(key, session.user.id, [{ clientMsgId: m.clientMsgId, conversationId: m.conversationId, originalBody: payload }], isCurrent);
      if (!sendCurrent()) return;
      if (!removed.includes(m.clientMsgId)) return;
      const accepted = result;
      setLive((s) => {
        const rows = s.messages[m.conversationId] ?? [];
        const prior = rows.find(
          (old) =>
            !old.pending &&
            (old.id === accepted.id || old.clientMsgId === m.clientMsgId),
        );
        const projection = mergeMessageProjection(prior, accepted);
        return {
          ...s,
          outbox: s.outbox.filter((o) => o.clientMsgId !== m.clientMsgId),
          messages: {
            ...s.messages,
            [m.conversationId]: [
              ...rows.filter((o) => o.clientMsgId !== m.clientMsgId),
              { ...projection, reactions: projection.reactions ?? [] },
            ],
          },
        };
      });
      setOnline(true);
    } catch (error) {
      if (!sendCurrent()) return;
      if (error instanceof ApiError) {
        setOnline(true);
        if (error.code === "RESTORE_EPOCH_CHANGED") {
          invalidateSession();
          setNotice(t("restoreChanged"));
        } else if (
          [
            "SEND_ADMISSION_CHANGED",
            "MENTION_ALL_FORBIDDEN",
            "MENTION_FORBIDDEN",
            "MENTION_MEMBER_REQUIRED",
            "QUALIFICATION_CHANGED",
            "FORWARD_SOURCE_CHANGED",
            "FORWARD_ADMISSION_CHANGED",
            "FORWARD_SOURCE_UNAVAILABLE",
            "INVALID_FORWARD",
            "FORWARD_PROTOCOL_REQUIRED",
            "FORWARD_DEPTH_LIMIT",
          ].includes(error.code) ||
          (m.type === "forward" &&
            ["NOT_A_MEMBER", "HISTORY_NOT_ALLOWED"].includes(error.code))
        ) {
          const key = namespace.current;
          await freezeOutboxMessage(key, m.clientMsgId, error.code);
          if (!sendCurrent() || key !== namespace.current)
            return;
          setLive((space) => ({
            ...space,
            outbox: space.outbox.map((message) =>
              message.clientMsgId === m.clientMsgId
                ? { ...message, frozen: true, freezeReason: error.code }
                : message,
            ),
            messages: {
              ...space.messages,
              [m.conversationId]: (
                space.messages[m.conversationId] ?? []
              ).filter(
                (message) =>
                  !message.pending || message.clientMsgId !== m.clientMsgId,
              ),
            },
          }));
          setNotice(t("sendAdmissionChanged"));
          void sync();
        } else report(error);
      } else {
        setOnline(false);
        setNotice(t("unknownResult"));
      }
    }
  };
  const send = async (
    e?: FormEvent,
    attachmentId?: string,
    type = "text",
    text?: string,
    onQueued?: () => void,
  ) => {
    e?.preventDefault();
    if (conversation?.readOnly) {
      setNotice(t("readOnlyConversation"));
      return;
    }
    if (!conversation || !me || busy || (!draft.trim() && !attachmentId))
      return;
    if (!demo && !conversation.sendAdmission) {
      setNotice(t("sendPermissionUnavailable"));
      void sync();
      return;
    }
    if (editing) {
      if (demo) {
        await mutate(editing, "edit", draft);
        setEditing(null);
        updateDraft("");
      } else {
        try {
          await api(
            "/messages/" + editing.id,
            json("PATCH", {
              text: draft,
              expectedVersion: editing.version,
              requestId: uid(),
            }),
          );
          setEditing(null);
          updateDraft("");
          void sync();
        } catch (e) {
          report(e);
        }
      }
      return;
    }
    const now = new Date().toISOString();
    const m: Message = {
      id: uid(),
      conversationId: selected,
      senderId: me.id,
      clientMsgId: uid(),
      type,
      text: text ?? draft,
      attachmentId,
      replyToId: reply?.id,
      createdAt: now,
      updatedAt: now,
      version: "1",
      sequence: String(Date.now()),
      deleted: false,
      reactions: [],
      pinned: false,
      favorite: false,
      pending: !demo,
      restoreEpoch: session?.restoreEpoch ?? "1",
      sendAdmission: conversation.sendAdmission,
      mentionedUserIds: mentionDraft.userIds.length
        ? [...mentionDraft.userIds]
        : undefined,
      mentionAll: mentionDraft.all || undefined,
    };
    if (demo) {
      setReply(null);
      setSample((s) => ({
        ...s,
        drafts: { ...s.drafts, [selected]: "" },
        mentionDrafts: {
          ...s.mentionDrafts,
          [selected]: { userIds: [], all: false },
        },
        messages: {
          ...s.messages,
          [selected]: [...(s.messages[selected] ?? []), m],
        },
      }));
      return;
    }
    if (sending.current) return;
    sending.current = true;
    const generation = changing.current;
    const latest = liveRef.current;
    const next = {
      ...latest,
      drafts: { ...latest.drafts, [selected]: "" },
      mentionDrafts: {
        ...latest.mentionDrafts,
        [selected]: { userIds: [], all: false },
      },
      outbox: [...latest.outbox, m],
      messages: {
        ...latest.messages,
        [selected]: [...(latest.messages[selected] ?? []), m],
      },
    };
    try {
      await queueMessage(namespace.current, next, m);
      onQueued?.();
      if (generation !== changing.current) return;
      setReply((current) => (current?.id === m.replyToId ? null : current));
      setLive((current) => ({
        ...current,
        drafts: {
          ...current.drafts,
          [selected]:
            (current.drafts[selected] ?? "") === draft
              ? ""
              : current.drafts[selected],
        },
        mentionDrafts: {
          ...current.mentionDrafts,
          [selected]:
            JSON.stringify(
              current.mentionDrafts?.[selected] ?? { userIds: [], all: false },
            ) === JSON.stringify(mentionDraft)
              ? { userIds: [], all: false }
              : current.mentionDrafts![selected],
        },
        outbox: [
          ...current.outbox.filter((old) => old.clientMsgId !== m.clientMsgId),
          m,
        ],
        messages: {
          ...current.messages,
          [selected]: [
            ...(current.messages[selected] ?? []).filter(
              (old) => old.clientMsgId !== m.clientMsgId,
            ),
            m,
          ],
        },
      }));
      await writeMessage(m, generation);
    } catch {
      setNotice(t("failure"));
    } finally {
      sending.current = false;
    }
  };
  const retryOutbox = async () => {
    const generation = changing.current;
    for (const m of live.outbox) {
      if (!session || demo || generation !== changing.current) break;
      await writeMessage(m, generation);
    }
  };
  const recheckForward = async (message: Message) => {
    if (demo || !session || message.pending || message.deleted) return;
    const generation = changing.current, key = namespace.current;
    const forwardGeneration = forwardProjectionGeneration.current;
    const isCurrent = () => generation === changing.current && key === namespace.current;
    try {
      const refreshed = await refreshSyncForwardViews(liveRef.current, [message], api, isCurrent);
      const fresh = refreshed.messages[message.conversationId]?.find(row => row.id === message.id);
      if (!fresh || !isCurrent() || forwardGeneration !== forwardProjectionGeneration.current) return;
      setLive(previous => ({ ...previous, messages: { ...previous.messages, [message.conversationId]: (previous.messages[message.conversationId] ?? []).map(row => row.id === message.id && BigInt(row.version) <= BigInt(fresh.version) && !(row.deleted && !fresh.deleted) ? fresh : row) } }));
    } catch (error) { if (isCurrent()) report(error); }
  };
  const mutate = async (m: Message, action: string, value?: string) => {
    if (action === "announce") {
      setAnnouncementSource(m);
      setMore("");
      setModal("announcement");
      return;
    }
    if (action === "report") {
      if (demo || !features.reports) {
        setNotice(t("notSupported"));
        return;
      }
      setReportTarget({
        targetType: "message",
        targetId: m.id,
        label: messageText(m),
      });
      setMore("");
      setModal("reports");
      return;
    }
    if ((m.contentUnavailable || m.forwardUnavailable) && action !== "recall") {
      setNotice(t("contentUnavailable"));
      return;
    }
    setMore("");
    const capability = (
      {
        pin: "pins",
        favorite: "favorites",
        reaction: "reactions",
        forward: "forwarding",
      } as Record<string, string>
    )[action];
    if (capability && !enabled(capability)) {
      setNotice(t("notSupported"));
      return;
    }
    if (action === "reply") {
      localDraftRevision.current++;
      setReply(m);
      composer.current?.focus();
      return;
    }
    if (action === "edit-start") {
      localDraftRevision.current++;
      setEditing(m);
      updateDraft(m.text);
      composer.current?.focus();
      return;
    }
    if (action === "forward") {
      if (demo) {
        setForwarding(m);
        setModal("forward");
        return;
      }
      const generation = changing.current;
      try {
        const source = await api<Message>(
          "/messages/" + encodeURIComponent(m.id),
        );
        if (generation !== changing.current) return;
        if (
          source.deleted ||
          source.contentUnavailable ||
          source.forwardUnavailable
        ) {
          setNotice(t("contentUnavailable"));
          return;
        }
        if (
          (source.contentVersion ?? source.version) !==
          (m.contentVersion ?? m.version)
        ) {
          setLive((value) => ({
            ...value,
            messages: {
              ...value.messages,
              [source.conversationId]: (
                value.messages[source.conversationId] ?? []
              ).map((old) =>
                old.id === source.id
                  ? mergeMessageProjection(old, source)
                  : old,
              ),
            },
          }));
          setNotice(t("forwardSourceUpdated"));
          return;
        }
        if (!source.forwardAdmission || !source.contentVersion) {
          setNotice(t("notSupported"));
          return;
        }
        setForwarding(source);
        setModal("forward");
      } catch (error) {
        if (generation === changing.current) report(error);
      }
      return;
    }
    if (action === "copy") {
      await navigator.clipboard.writeText(m.text);
      setNotice(t("copied"));
      return;
    }
    if (action === "recall" && !confirm(t("confirmRecall"))) return;
    const local = () =>
      setSample((s) => ({
        ...s,
        messages: {
          ...s.messages,
          [m.conversationId]: (s.messages[m.conversationId] ?? []).map((x) =>
            x.id !== m.id
              ? x
              : action === "pin"
                ? { ...x, pinned: !x.pinned }
                : action === "favorite"
                  ? { ...x, favorite: !x.favorite }
                  : action === "recall"
                    ? { ...x, deleted: true, text: "" }
                    : action === "edit"
                      ? {
                          ...x,
                          text: value ?? x.text,
                          version: String(Number(x.version) + 1),
                        }
                      : action === "reaction"
                        ? {
                            ...x,
                            reactions: x.reactions.length
                              ? []
                              : [{ emoji: "👍", userId: me?.id }],
                          }
                        : x,
          ),
        },
      }));
    if (demo) {
      local();
      return;
    }
    try {
      const base = "/messages/" + m.id;
      await api(
        base +
          (action === "pin"
            ? "/pin"
            : action === "favorite"
              ? "/favorite"
              : action === "reaction"
                ? "/reactions"
                : ""),
        json(
          action === "pin"
            ? m.pinned
              ? "DELETE"
              : "PUT"
            : action === "favorite"
              ? m.favorite
                ? "DELETE"
                : "PUT"
              : action === "recall"
                ? "DELETE"
                : action === "edit"
                  ? "PATCH"
                  : "POST",
          action === "edit"
            ? { text: value, expectedVersion: m.version, requestId: uid() }
            : action === "reaction"
              ? { emoji: "👍" }
              : undefined,
        ),
      );
      await sync();
    } catch (e) {
      report(e);
    }
  };
  const captureAttachmentContext = (): AttachmentSendContext | undefined => {
    const snapshot = authSnapshot(), auth = snapshot.auth, key = namespace.current, generation = changing.current, selection = currentSelection.current;
    if (demo || !session || !auth?.userId || !auth.deviceId || !auth.familyId || !key) return;
    const identity = { namespace: key, userId: auth.userId, deviceId: auth.deviceId, familyId: auth.familyId, restoreEpoch: auth.restoreEpoch };
    const isCurrent = () => generation === changing.current && key === namespace.current && selection === currentSelection.current && authSnapshotCurrent(snapshot);
    const context: AttachmentSendContext = {
      identity, isCurrent,
      enqueue: async entry => {
        if (!isCurrent() || featureState.current.media !== true) throw Error("ATTACHMENT_CONTEXT_CHANGED");
        const message = entry.message, latest = liveRef.current;
        const next = { ...latest, outbox: [...latest.outbox.filter(value => value.clientMsgId !== message.clientMsgId), message], messages: { ...latest.messages, [message.conversationId]: [...(latest.messages[message.conversationId] ?? []).filter(value => value.clientMsgId !== message.clientMsgId), message] } };
        await queueMessage(key, next, message, linkAttachmentEnqueue(context, entry, message));
        if (!isCurrent()) return;
        setLive(current => ({ ...current, outbox: [...current.outbox.filter(value => value.clientMsgId !== message.clientMsgId), message], messages: { ...current.messages, [message.conversationId]: [...(current.messages[message.conversationId] ?? []).filter(value => value.clientMsgId !== message.clientMsgId), message] } }));
        setAttachmentRevision(value => value + 1);
        await writeMessage(message, generation);
      },
      accepted: async (entry, message) => {
        if (!isCurrent()) throw Error("ATTACHMENT_CONTEXT_CHANGED");
        await acceptAttachmentMessage(context, message);
        const removed = await acknowledgeOriginalOutbox(key, identity.userId, [{ clientMsgId: entry.clientMsgId, conversationId: entry.message.conversationId, originalBody: messagePayload(entry.message) }], isCurrent);
        if (!isCurrent()) return;
        if (removed.includes(entry.clientMsgId)) setLive(current => ({ ...current, outbox: current.outbox.filter(value => value.clientMsgId !== entry.clientMsgId), messages: { ...current.messages, [message.conversationId]: [...(current.messages[message.conversationId] ?? []).filter(value => value.clientMsgId !== entry.clientMsgId), message] } }));
        setAttachmentRevision(value => value + 1);
      },
    };
    return context;
  };
  const upload = async (file: File, recordingCurrent: () => boolean = () => true): Promise<boolean> => {
    const generation = changing.current, key = namespace.current, selection = selected, snapshot = authSnapshot();
    const isCurrent = () => generation === changing.current && key === namespace.current && currentSelection.current === selection && recordingCurrent() && (!session?.familyId || authSnapshotCurrent(snapshot));
    if (!isCurrent() || !canSend || editing || busy) return false;
    if (file.size > 64 * 1024 * 1024) {
      setNotice(t("attachmentLimit"));
      return false;
    }
    if (demo) {
      const a = URL.createObjectURL(file);
      const now = new Date().toISOString();
      const m: Message = {
        id: uid(),
        conversationId: selected,
        senderId: me!.id,
        clientMsgId: uid(),
        type: file.type.startsWith("image/")
          ? "image"
          : file.type.startsWith("audio/")
            ? "audio"
            : file.type.startsWith("video/")
              ? "video"
              : "file",
        text: file.name,
        attachmentId: a,
        createdAt: now,
        updatedAt: now,
        version: "1",
        sequence: String(Date.now()),
        deleted: false,
        reactions: [],
        pinned: false,
        favorite: false,
      };
      setSample((s) => ({
        ...s,
        messages: {
          ...s.messages,
          [selected]: [...(s.messages[selected] ?? []), m],
        },
      }));
      return true;
    }
    const context = captureAttachmentContext();
    if (!context || !conversation || !me || !conversation.sendAdmission) return false;
    setBusy(true);
    try {
      const now = new Date().toISOString(), message: Message = { id: uid(), clientMsgId: uid(), conversationId: selected, senderId: me.id, type: file.type.startsWith("image/") ? "image" : file.type.startsWith("audio/") ? "audio" : file.type.startsWith("video/") ? "video" : "file", text: file.name, createdAt: now, updatedAt: now, version: "1", sequence: "0", deleted: false, reactions: [], pinned: false, favorite: false, pending: true, restoreEpoch: context.identity.restoreEpoch, sendAdmission: conversation.sendAdmission, replyToId: reply?.id, mentionedUserIds: [...mentionDraft.userIds], mentionAll: mentionDraft.all || undefined };
      const preparation = new AttachmentSendActions({ ...context, isCurrent: () => context.isCurrent() && recordingCurrent() });
      const entry = await preparation.prepare(file, message, reply ? reply.contentVersion ?? reply.version : undefined);
      if (!context.isCurrent()) return true;
      setAttachmentRevision(value => value + 1); setNotice(attachmentLabel(locale, "retained"));
      // The strict Blob/manifest handoff owns subsequent work; closing the RAM preview cannot cancel it.
      void new AttachmentSendActions(context).run(entry.requestId, true).catch(() => {}).finally(() => { if (context.isCurrent()) setAttachmentRevision(value => value + 1); });
      return true;
    } catch (e) {
      if (isCurrent()) report(e);
      return false;
    } finally {
      if (generation === changing.current && key === namespace.current) setBusy(false);
    }
  };
  const openConversation = (c: Conversation) => {
    readingBottom.current = true;
    setSpace((value) => ({
      ...value,
      unread: { ...value.unread, [c.id]: [] },
      mentionUnread: { ...value.mentionUnread, [c.id]: [] },
    }));
    setSelected(c.id);
    setSection("messages");
    setMobileOpen(true);
  };
  const createConversation = async (
    ids: string[],
    kind = "direct",
    title = "",
  ) => {
    if (demo) {
      const c: Conversation = {
        id: uid(),
        kind: kind as "direct",
        title:
          title ||
          demoUsers.find((u) => u.id === ids[0])?.displayName ||
          t("note"),
        ownerId: me!.id,
        memberIds: [me!.id, ...ids],
        updatedAt: new Date().toISOString(),
        cursor: "0",
      };
      setSample((s) => ({ ...s, conversations: [c, ...s.conversations] }));
      openConversation(c);
      setModal("");
      return;
    }
    try {
      const c = await api<Conversation>(
        "/conversations",
        json("POST", { kind, title, memberIds: ids, requestId: uid() }),
      );
      setLive((s) => ({
        ...s,
        conversations: [c, ...s.conversations.filter((x) => x.id !== c.id)],
      }));
      openConversation(c);
      setModal("");
    } catch (e) {
      report(e);
    }
  };
  const findPeople = async (value: string) => {
    const request = ++peopleRequest.current;
    const generation = changing.current;
    setPeopleQueried(true);
    if (demo) {
      setPeople(
        demoUsers.filter(
          (u) =>
            u.id !== me?.id &&
            (u.displayName.includes(value) || u.username.includes(value)),
        ),
      );
      return;
    }
    try {
      const r = await api<{ items: User[] }>(
        "/users?q=" + encodeURIComponent(value),
      );
      if (request === peopleRequest.current && generation === changing.current)
        setPeople(r.items ?? []);
    } catch (e) {
      if (request === peopleRequest.current) report(e);
    }
  };
  const refreshMembers = async (id = selected, append = false) => {
    const generation = changing.current;
    if (demo) {
      setMemberCursor("");
      setMembers(
        demoUsers
          .filter((user) => conversation?.memberIds.includes(user.id))
          .map((user) => ({ ...user, membershipGeneration: "1" })),
      );
      return;
    }
    try {
      const after = append ? memberCursor : "";
      const result = await api<{ items: User[]; nextCursor?: string }>(
        "/conversations/" + encodeURIComponent(id) + "/members?limit=50" + (after ? "&after=" + encodeURIComponent(after) : ""),
      );
      if (generation === changing.current && currentSelection.current === id) {
        setMembers(rows => append ? [...rows, ...(result.items ?? []).filter(item => !rows.some(row => row.id === item.id))] : result.items ?? []);
        setMemberCursor(result.nextCursor ?? "");
      }
    } catch (error) {
      if (generation === changing.current) report(error);
    }
  };
  const showMembers = async () => {
    setDetails((value) => !value);
    await refreshMembers();
  };
  const groupAction = async (
    action: GroupAction,
    target?: User,
    title?: string,
  ) => {
    const current = conversation;
    if (
      !current ||
      current.kind !== "group" ||
      current.readOnly ||
      grouping.current
    )
      return;
    if (
      (action === "remove" || action === "leave") &&
      !target?.membershipGeneration
    ) {
      setNotice(t("reload"));
      return;
    }
    const generation = changing.current,
      key = namespace.current;
    grouping.current = true;
    setBusy(true);
    try {
      if (demo) {
        setSample((space) => ({
          ...space,
          conversations: space.conversations
            .map((c) =>
              c.id !== current.id
                ? c
                : action === "rename"
                  ? { ...c, title: title ?? c.title }
                  : action === "transfer"
                    ? { ...c, ownerId: target!.id }
                    : {
                        ...c,
                        memberIds: c.memberIds.filter(
                          (id) => id !== target!.id,
                        ),
                      },
            )
            .filter((c) => !(action === "leave" && c.id === current.id)),
        }));
        if (action === "leave") {
          setSelected("");
          setDetails(false);
        } else if (action === "remove")
          setMembers((rows) => rows.filter((user) => user.id !== target!.id));
      } else {
        const path = "/conversations/" + encodeURIComponent(current.id);
        if (action === "transfer") throw new SyncProtocolError("GROUP_PERMISSION_CHANGED");
        if (action === "rename") {
          const updated = await api<Conversation>(
            path,
            json(
              "PATCH",
              { title },
            ),
          );
          if (generation !== changing.current) return;
          setLive((space) => ({
            ...space,
            conversations: space.conversations.map((c) =>
              c.id === updated.id ? updated : c,
            ),
          }));
          await refreshMembers(current.id);
        } else {
          await api(
            path +
              "/members/" +
              encodeURIComponent(target!.id) +
              "?membershipGeneration=" +
              encodeURIComponent(target!.membershipGeneration!),
            json("DELETE"),
          );
          if (generation !== changing.current) return;
          if (action === "leave") {
            const latest = liveRef.current,
              messages = { ...latest.messages },
              cursors = { ...latest.cursors },
              drafts = { ...latest.drafts };
            delete messages[current.id];
            delete cursors[current.id];
            delete drafts[current.id];
            const next = {
              ...latest,
              conversations: latest.conversations.filter(
                (c) => c.id !== current.id,
              ),
              messages,
              cursors,
              drafts,
              outbox: latest.outbox.map((m) =>
                m.conversationId === current.id ? { ...m, frozen: true } : m,
              ),
            };
            await freezeConversation(key, current.id, next);
            if (generation !== changing.current) return;
            changing.current++;
            resetRequestContext();
            setLive(next);
            setSelected("");
            setDetails(false);
            setMembers([]);
          } else {
            await refreshMembers(current.id);
            void sync();
          }
        }
      }
      setNotice(t("saved"));
    } catch (error) {
      if (generation === changing.current) {
        report(error);
        void sync();
        void refreshMembers(current.id);
      }
    } finally {
      grouping.current = false;
      if (demo || namespace.current === key) setBusy(false);
    }
  };
  const freezeDirectQueue = async (conversationId: string) => {
    const key = namespace.current;
    const generation = ++changing.current;
    resetRequestContext();
    const latest = liveRef.current;
    const next = {
      ...latest,
      outbox: latest.outbox.map((message) =>
        message.conversationId === conversationId
          ? { ...message, frozen: true }
          : message,
      ),
      messages: {
        ...latest.messages,
        [conversationId]: (latest.messages[conversationId] ?? []).filter(
          (message) => !message.pending,
        ),
      },
    };
    await freezeConversation(key, conversationId, next);
    if (generation !== changing.current || key !== namespace.current)
      throw Error("Account changed");
    liveRef.current = next;
    setLive(next);
  };
  const auth = async (e: FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    setBusy(true);
    setNotice("");
    const data = new FormData(e.currentTarget);
    const requestId = authRequest.current || (authRequest.current = uid());
    const username = String(data.get("username") ?? "").trim();
    const password = String(data.get("password") ?? "");
    const submitter = (e.nativeEvent as SubmitEvent).submitter;
    const freshProof = submitter instanceof HTMLButtonElement && submitter.value === "fresh-proof";
    const submittedModal = modalGeneration.current;
    await withAuthLifecycle(async () => {
    let initial: InitialAuth | undefined;
    try {
      if (submittedModal !== modalGeneration.current) return;
      if (modal === "recovery") {
        await api(
          "/auth/recover",
          json("POST", {
            username,
            password,
            code: data.get("code"),
            requestId,
          }),
        );
        setNotice(t("saved"));
        setModal("signIn");
        return;
      }
      if (authProtocol() || hasAuthResponsibility()) {
        const live = await api<{ deploymentId: string; features?: Record<string, boolean> }>("/bootstrap", {}, 3000);
        if (live.deploymentId !== deployment) throw new ApiError(409, "AUTH_CONTEXT_CHANGED", "Instance identity changed");
        publishFeatures(live.features ?? {});
        if (!authProtocol() && hasAuthResponsibility()) throw Error("AUTH_RECOVERY_REQUIRED");
      }
      if (authProtocol()) {
        const previous = pendingInitial(username);
        if (!freshProof && previous && !previous.expired && !hasInitialProof(previous.requestId) && !window.confirm(t("authOriginalProofConfirm"))) return;
        changing.current++;
        resetRequestContext();
        namespace.current = "";
        setSession(null);
        setLive(emptySpace());
        setSelected("");
        setReply(null);
        setEditing(null);
        setForwarding(null);
        setAnnouncementSource(null);
        initial = prepareInitial(modal === "register" ? "register" : "login", username, freshProof);
        retainInitialProof(initial.requestId, JSON.stringify({ username: initial.username, password, otp: String(data.get("otp") ?? ""), deviceName: initial.deviceName }));
      }
      let s = await api<Session>(
        "/auth/" + (initial?.kind ?? (modal === "register" ? "register" : "login")),
        json("POST", {
          username: initial?.username ?? username,
          password,
          deviceName: initial?.deviceName ?? navigator.platform + " Web",
          requestId: initial?.requestId ?? requestId,
          ...(initial?.previousFamilyId ? { previousFamilyId: initial.previousFamilyId } : {}),
          ...(initial?.previousRequestId ? { previousRequestId: initial.previousRequestId } : {}),
          ...(initial?.previousRequestIds ? { previousRequestIds: initial.previousRequestIds } : {}),
          ...(data.get("otp") ? { otp: String(data.get("otp")) } : {}),
        }),
      );
      clearCredentials();
      await api("/auth/confirm", json("POST", {}));
      s = await api<Session>("/auth/context");
      if (data.get("displayName")) {
        try {
          s.user = await api<User>(
            "/me",
            json("PATCH", { displayName: String(data.get("displayName")) }),
          );
        } catch (e) {
          report(e);
        }
      }
      if (submittedModal !== modalGeneration.current) throw new ApiError(409, "AUTH_REQUEST_ABORTED", "Authentication view changed");
      await activate(s, deployment);
      setModal("");
      setOnline(true);
      authRequest.current = "";
    } catch (e) {
      if (
        e instanceof ApiError &&
        ["MFA_REQUIRED", "MFA_INVALID", "MFA_REPLAY"].includes(e.code)
      ) {
        setRequiresOTP(true);
        setNotice(t("otpRequired"));
      } else if ((e instanceof Error && e.message === "AUTH_RECOVERY_REQUIRED") || (e instanceof ApiError && e.code === "AUTH_RECOVERY_REQUIRED")) {
        setNotice(t("authRecoveryUnavailable"));
      } else if (e instanceof Error && e.message === "AUTH_ORIGINAL_PROOF_CHANGED") {
        setNotice(t("authOriginalProofChanged"));
      } else if (e instanceof ApiError && e.code === "DELIVERY_EXPIRED" && initial) {
        expireInitial(initial.requestId);
        setNotice(t("authDeliveryExpired"));
      } else if (initial && !(e instanceof ApiError && e.status < 500)) {
        setNotice(t("authInitialUnknown"));
      } else report(e);
    } finally {
      setBusy(false);
    }
    }).catch(error => {
      report(error);
      setBusy(false);
    });
  };
  const settleAvatar = async (
    result: AvatarResult,
    key: string,
    generation: number,
  ) => {
    if (!["applied", "cancelled", "superseded"].includes(result.status)) {
      setNotice(t("working"));
      return;
    }
    const user = await api<User>("/me");
    if (generation !== changing.current) return;
    await clearIntent(key, "avatar");
    if (generation !== changing.current) return;
    setSession((s) => (s ? { ...s, user } : s));
    setAvatarPending(null);
    setNotice(t("avatarChecked"));
  };
  const inspectAvatar = async (
    intent: AvatarIntent,
    key = namespace.current,
  ) => {
    const generation = changing.current;
    if (intent.restoreEpoch !== (session?.restoreEpoch ?? "1")) {
      await saveIntent(key, "avatar-archive-" + intent.restoreEpoch, intent);
      await clearIntent(key, "avatar");
      if (generation === changing.current) {
        setAvatarPending(null);
        setNotice(t("restoreChanged"));
      }
      return;
    }
    try {
      const result = await api<AvatarResult>(
        "/me/avatar/operations/" + encodeURIComponent(intent.requestId),
      );
      if (generation !== changing.current) return;
      await settleAvatar(result, key, generation);
    } catch (error) {
      if (
        generation === changing.current &&
        !(error instanceof ApiError && error.status === 404)
      )
        report(error);
    }
  };
  useEffect(() => {
    if (!session || demo || !namespace.current) return;
    const key = namespace.current,
      generation = changing.current;
    void loadIntent<AvatarIntent>(key, "avatar")
      .then((intent) => {
        if (generation !== changing.current) return;
        setAvatarPending(intent ?? null);
        if (intent) void inspectAvatar(intent, key);
      })
      .catch(report);
  }, [session?.deviceId, demo]);
  const publishAvatar = async (file?: File) => {
    if (demo || !session || busy) return;
    const generation = changing.current,
      key = namespace.current;
    let intent = avatarPending;
    if (!intent) {
      if (!file) return;
      if (file.size > 2 * 1024 * 1024) {
        setNotice(t("avatarLimit"));
        return;
      }
      intent = {
        namespace: key,
        requestId: uid(),
        expectedVersion: session.user.profileVersion ?? "1",
        restoreEpoch: session.restoreEpoch ?? "1",
        file,
        name: file.name,
      };
      try {
        await saveIntent(key, "avatar", intent);
        if (generation !== changing.current) return;
        setAvatarPending(intent);
      } catch (error) {
        report(error);
        return;
      }
    }
    if (intent.namespace !== key) {
      setAvatarPending(null);
      setNotice(t("failure"));
      return;
    }
    if (intent.restoreEpoch !== (session.restoreEpoch ?? "1")) {
      await inspectAvatar(intent, key);
      return;
    }
    setBusy(true);
    try {
      const form = new FormData();
      form.append("file", intent.file, intent.name);
      form.append("requestId", intent.requestId);
      form.append("expectedVersion", intent.expectedVersion);
      const result = await api<AvatarResult>(
        "/me/avatar",
        { method: "POST", body: form },
        120000,
      );
      if (generation !== changing.current) return;
      await settleAvatar(result, key, generation);
    } catch (error) {
      if (generation === changing.current) {
        if (error instanceof ApiError && error.status < 500) {
          if ([400, 413, 415, 422].includes(error.status)) {
            await clearIntent(key, "avatar");
            setAvatarPending(null);
          }
          report(error);
        } else setNotice(t("unknownResult"));
      }
    } finally {
      setBusy(false);
    }
  };
  const signOut = async () => {
    try {
      await api("/auth/logout", json("POST", {}));
    } catch (e) {
      report(e);
      return;
    }
    changing.current++;
    resetRequestContext();
    namespace.current = "";
    setSession(null);
    setLive(emptySpace());
    setDemo(true);
    setSelected("demo-design");
    clearCredentials();
  };
  const displayUser = (id: string) =>
    id === me?.id
      ? me.displayName
      : (space.profiles?.[id]?.displayName ??
        [...contacts, ...members, ...(demo ? demoUsers : [])].find(
          (u) => u.id === id,
        )?.displayName ??
        id.slice(0, 8));
  const avatarFor = (id: string) => {
    const user =
      id === me?.id
        ? me
        : (space.profiles?.[id] ??
          [...contacts, ...members, ...people].find((user) => user.id === id));
    return user?.avatarThumbnailURL || user?.avatarURL;
  };
  const conversationAvatar = (c: Conversation) =>
    c.kind === "direct"
      ? avatarFor(c.memberIds.find((id) => id !== me?.id) ?? "")
      : c.kind === "self"
        ? avatarFor(me?.id ?? "")
        : undefined;
  const conversationLabel = (c: Conversation) =>
    c.title?.trim() ||
    (c.kind === "self"
      ? t("note")
      : c.kind === "group"
        ? t("group")
        : displayUser(c.memberIds.find((id) => id !== me?.id) ?? ""));
  const dayKey = (value: string) => {
    const d = new Date(value);
    return d.getFullYear() + "-" + d.getMonth() + "-" + d.getDate();
  };
  const date = (s: string) => {
    try {
      return new Intl.DateTimeFormat(locale, {
        hour: "2-digit",
        minute: "2-digit",
      }).format(new Date(s));
    } catch {
      return new Date(s).toLocaleTimeString();
    }
  };
  const filteredContacts = contacts.filter((u) =>
    u.displayName.includes(query),
  );
  const messageText = (message?: Message) =>
    !message
      ? ""
      : message.deleted
        ? t("recall")
        : message.contentUnavailable || message.forwardUnavailable
          ? t("contentUnavailable")
          : message.type === "forward" && message.pending
            ? t("forwardPending")
            : message.text;
  const visible = space.conversations.filter((c) => {
    const view = personalStateRecord(personal, "conversation_view", c.id);
    return demo || features.sync_self_state !== true || chatFilter === "allChats" || (chatFilter === "archived" ? view?.archived === true : chatFilter === "hidden" ? view?.hidden === true : view?.archived !== true && view?.hidden !== true);
  }).filter((c) =>
    conversationLabel(c)
      .toLocaleLowerCase()
      .includes(query.toLocaleLowerCase()),
  ).sort((a, b) => {
    const left = personalStateRecord(personal, "conversation_view", a.id)?.pinnedRank;
    const right = personalStateRecord(personal, "conversation_view", b.id)?.pinnedRank;
    return (typeof left === "number" ? left : 1000000) - (typeof right === "number" ? right : 1000000) || b.updatedAt.localeCompare(a.updatedAt);
  });
  const empty = (label: string, icon: ReactNode, action?: () => void) => (
    <div className="empty">
      <div className="empty-icon">{icon}</div>
      <h2>{label}</h2>
      <p>{t("welcomeText")}</p>
      <button className="secondary" onClick={action ?? (() => void sync())}>
        <RefreshCw size={16} />
        {t("reload")}
      </button>
    </div>
  );
  const chooseLookup = async (candidate: Message) => {
    const generation = changing.current;
    let target: Conversation, message: Message;
    try {
      [target, message] = demo
        ? [
            space.conversations.find(
              (item) => item.id === candidate.conversationId,
            )!,
            candidate,
          ]
        : await Promise.all([
            api<Conversation>(
              "/conversations/" + encodeURIComponent(candidate.conversationId),
            ),
            api<Message>("/messages/" + encodeURIComponent(candidate.id)),
          ]);
    } catch (error) {
      if (candidate.forwarded && generation === changing.current)
        setSpace((value) => ({
          ...value,
          messages: {
            ...value.messages,
            [candidate.conversationId]: (
              value.messages[candidate.conversationId] ?? []
            ).map((old) =>
              old.id === candidate.id ? unavailableForward(old) : old,
            ),
          },
        }));
      throw error;
    }
    if (generation !== changing.current || !target) return;
    readingBottom.current = false;
    setSpace((value) => {
      const previous = value.messages[message.conversationId] ?? [];
      const newest = previous.find((item) => item.id === message.id);
      const result =
        newest &&
        (BigInt(newest.version) > BigInt(message.version) ||
          (newest.forwardUnavailable &&
            BigInt(newest.version) === BigInt(message.version)))
          ? newest
          : demo
            ? message
            : mergeMessageProjection(newest, message);
      return {
        ...value,
        conversations: [
          target,
          ...value.conversations.filter((item) => item.id !== target.id),
        ],
        messages: {
          ...value.messages,
          [message.conversationId]: [
            ...previous.filter((item) => item.id !== message.id),
            result,
          ].sort(
            (a, b) =>
              Date.parse(a.createdAt) - Date.parse(b.createdAt) ||
              a.id.localeCompare(b.id),
          ),
        },
      };
    });
    setSelected(target.id);
    setSection("messages");
    setMobileOpen(true);
    closeModal();
    requestAnimationFrame(() =>
      requestAnimationFrame(() =>
        document
          .getElementById("message-" + message.id)
          ?.scrollIntoView({ block: "center" }),
      ),
    );
  };
  const forwardTo = async (target: Conversation) => {
    const source = forwarding;
    if (!source || sending.current || !me) return;
    if (demo) {
      const copy: Message = {
        ...source,
        id: uid(),
        clientMsgId: uid(),
        conversationId: target.id,
        senderId: me.id,
        createdAt: new Date().toISOString(),
        updatedAt: new Date().toISOString(),
        version: "1",
        contentVersion: "1",
        reactions: [],
        pinned: false,
        favorite: false,
        forwarded: true,
      };
      setSample((value) => ({
        ...value,
        messages: {
          ...value.messages,
          [target.id]: [...(value.messages[target.id] ?? []), copy],
        },
      }));
      closeModal();
      openConversation(target);
      return;
    }
    if (!source.forwardAdmission || !source.contentVersion || !session) {
      setNotice(t("notSupported"));
      return;
    }
    if (
      liveRef.current.outbox.some(
        (message) =>
          message.type === "forward" &&
          message.conversationId === target.id &&
          message.forwardOfId === source.id &&
          !message.frozen,
      )
    ) {
      setNotice(t("unknownResult"));
      return;
    }
    const generation = changing.current,
      key = namespace.current;
    sending.current = true;
    setBusy(true);
    try {
      const current = await api<Conversation>(
        "/conversations/" + encodeURIComponent(target.id),
      );
      if (generation !== changing.current) return;
      if (current.readOnly || !current.sendAdmission) {
        setNotice(t("sendPermissionUnavailable"));
        return;
      }
      const now = new Date().toISOString();
      const message: Message = {
        id: uid(),
        clientMsgId: uid(),
        conversationId: current.id,
        senderId: session.user.id,
        type: "forward",
        text: "",
        createdAt: now,
        updatedAt: now,
        version: "1",
        sequence: "0",
        deleted: false,
        reactions: [],
        pinned: false,
        favorite: false,
        pending: true,
        forwarded: true,
        restoreEpoch: session.restoreEpoch ?? "1",
        sendAdmission: current.sendAdmission,
        forwardOfId: source.id,
        forwardSourceVersion: source.contentVersion,
        forwardAdmission: source.forwardAdmission,
      };
      const latest = liveRef.current;
      await queueMessage(
        key,
        {
          ...latest,
          conversations: [
            current,
            ...latest.conversations.filter((item) => item.id !== current.id),
          ],
          outbox: [...latest.outbox, message],
          messages: {
            ...latest.messages,
            [current.id]: [...(latest.messages[current.id] ?? []), message],
          },
        },
        message,
      );
      if (generation !== changing.current || key !== namespace.current) return;
      setLive((value) => ({
        ...value,
        conversations: [
          current,
          ...value.conversations.filter((item) => item.id !== current.id),
        ],
        outbox: [
          ...value.outbox.filter(
            (item) => item.clientMsgId !== message.clientMsgId,
          ),
          message,
        ],
        messages: {
          ...value.messages,
          [current.id]: [
            ...(value.messages[current.id] ?? []).filter(
              (item) => item.clientMsgId !== message.clientMsgId,
            ),
            message,
          ],
        },
      }));
      closeModal();
      openConversation(current);
      await writeMessage(message, generation);
    } catch (error) {
      if (generation === changing.current) report(error);
    } finally {
      sending.current = false;
      if (key === namespace.current) setBusy(false);
    }
  };
  const closeModal = () => {
    setModal("");
    setGenerated([]);
    setForwarding(null);
    setPeople([]);
    setPeopleQueried(false);
    peopleRequest.current++;
    setPick([]);
  };
  return (
    <div className={"app " + (mobileOpen ? "thread-open" : "")}>
      <a className="skip-link" href="#conversation">
        {t("messages")}
      </a>
      <nav className="rail" aria-label="JobsIM">
        <img
          className="brand-mark"
          src="/Logo.svg"
          alt="JobsIM"
          width="38"
          height="38"
        />
        <div className="rail-links">
          {[
            ["messages", MessageCircle],
            ["contacts", Users],
            ["favorites", Bookmark],
          ].map(([key, Icon]) => (
            <IconButton
              key={String(key)}
              label={t(String(key))}
              active={section === key}
              current={section === key}
              onClick={() => {
                setSection(String(key));
                setMobileOpen(false);
              }}
            >
              <Icon size={22} />
            </IconButton>
          ))}
        </div>
        <div className="rail-bottom">
          <IconButton
            label={t("settings")}
            active={section === "settings"}
            current={section === "settings"}
            onClick={() => {
              setSection("settings");
              setMobileOpen(true);
            }}
          >
            <Settings size={22} />
          </IconButton>
          <button
            className="me-avatar"
            aria-label={t("profile")}
            onClick={() => setModal(session ? "profile" : "signIn")}
          >
            <Avatar
              name={me?.displayName ?? "Jobs"}
              src={me?.avatarThumbnailURL || me?.avatarURL}
              size={36}
            />
          </button>
        </div>
      </nav>
      <aside className="sidebar">
        <header className="sidebar-heading">
          <div>
            <div className="brand-name">
              JobsIM
              <span className="brand-dot" />
            </div>
            <h1>{t(section === "settings" ? "messages" : section)}</h1>
          </div>
          <IconButton
            label={t("newChat")}
            onClick={() => {
              setModal("newChat");
              setPeople(
                demo ? demoUsers.filter((u) => u.id !== me?.id) : contacts,
              );
            }}
          >
            <Plus size={23} />
          </IconButton>
        </header>
        <label className="search">
          <Search size={17} />
          <input
            aria-label={t("search")}
            placeholder={t("search")}
            value={query}
            onChange={(e) => setQuery(e.target.value)}
          />
          <kbd>⌘ K</kbd>
        </label>
        <div className="space-label">
          <span
            className={"status-dot " + (online && !demo ? "connected" : "")}
          />
          <span>{demo ? t("demo") : t(online ? "online" : "offline")}</span>
          <button
            onClick={() => {
              if (section === "favorites")
                setLookupReload((value) => value + 1);
              void (demo
                ? bootstrap()
                : Promise.all([sync(), refreshContacts()]));
            }}
            aria-label={t("reload")}
          >
            <RefreshCw size={14} />
          </button>
        </div>
        {!demo && features.sync_self_state === true && section !== "contacts" && section !== "favorites" && <label className="chat-filter">
          <span>{selfLabel(locale, "myState")}</span>
          <select value={chatFilter} onChange={event => setChatFilter(event.target.value)} aria-label={selfLabel(locale, "allChats")}>
            {["inbox", "allChats", "archived", "hidden"].map(filter => <option key={filter} value={filter}>{selfLabel(locale, filter)}</option>)}
          </select>
        </label>}
        <div className="conversation-list">
          {section === "contacts" ? (
            <>
              <button
                className="new-contact"
                onClick={() => {
                  setModal("newChat");
                  void findPeople("");
                }}
              >
                <UserRound size={19} />
                {t("add")}
              </button>
              {filteredContacts.map((u) => (
                <button
                  key={u.id}
                  className="contact-row"
                  onClick={() => void createConversation([u.id])}
                >
                  <Avatar
                    name={u.displayName}
                    src={u.avatarThumbnailURL || u.avatarURL}
                  />
                  <div>
                    <strong>{u.displayName}</strong>
                    <small>@{u.username}</small>
                  </div>
                  <ChevronRight size={16} />
                </button>
              ))}
              {requests.map((r) => (
                <div className="contact-request" key={r.id}>
                  <span>
                    {r.requester?.displayName ||
                      r.requester?.username ||
                      r.displayName ||
                      r.requesterId ||
                      r.id.slice(0, 8)}
                  </span>
                  <button onClick={() => void resolveRequest(r.id, true)}>
                    {t("accept")}
                  </button>
                  <button onClick={() => void resolveRequest(r.id, false)}>
                    {t("reject")}
                  </button>
                </div>
              ))}
              {!filteredContacts.length &&
                empty(t(query ? "noResults" : "emptyContacts"), <Users />)}
            </>
          ) : section === "favorites" ? (
            <MessageLookup
              key={
                String(demo) +
                "|" +
                namespace.current +
                "|favorites|" +
                lookupReload
              }
              kind="favorites"
              demo={demo}
              cached={Object.values(space.messages).flat()}
              filter={query}
              t={t}
              text={messageText}
              date={date}
              onChoose={chooseLookup}
              onError={report}
            />
          ) : (
            <>
              {visible.map((c) => {
                const ms = space.messages[c.id] ?? [];
                const last = ms.at(-1);
                const selfView = personalStateRecord(personal, "conversation_view", c.id);
                const sourceCount = readCounts.namespace === namespace.current && readCounts.counts[c.id]?.localRevision === space.syncRevision ? readCounts.counts[c.id] : undefined;
                const estimated = space.unread?.[c.id]?.length ?? 0;
                const unreadLabel = !demo && features.sync_self_state === true ? sourceCount?.count ?? (estimated ? "~" + estimated : "") : estimated ? String(estimated) : "";
                return (
                  <button
                    className={
                      "conversation-row " +
                      (selected === c.id ? "selected" : "")
                    }
                    key={c.id}
                    aria-current={selected === c.id ? "true" : undefined}
                    onClick={() => openConversation(c)}
                  >
                    <Avatar
                      name={conversationLabel(c)}
                      src={conversationAvatar(c)}
                      group={c.kind === "group"}
                    />
                    <div className="conversation-copy">
                      <div className="conversation-title">
                        <strong>{conversationLabel(c)}</strong>
                        <time>{date(last?.createdAt ?? c.updatedAt)}</time>
                      </div>
                      <div className="conversation-preview">
                        <span>
                          {last?.deleted
                            ? t("recall")
                            : messageText(last) || t("emptyMessages")}
                        </span>
                        {!!space.mentionUnread?.[c.id]?.length && (
                          <span
                            className="mention-badge"
                            aria-label={
                              t("mentionsUnread") +
                              " " +
                              space.mentionUnread[c.id].length
                            }
                          >
                            @
                          </span>
                        )}
                        {unreadLabel && unreadLabel !== "0" && (
                          <span
                            className="unread"
                            aria-label={t("unread") + " " + unreadLabel}
                          >
                            {unreadLabel}
                          </span>
                        )}
                        {selfView?.manualUnread === true && <span className="manual-unread-dot" aria-label={selfLabel(locale, "manualUnread")}>●</span>}
                        {ms.some((m) => m.pending && !m.frozen) &&
                          !space.unread?.[c.id]?.length && (
                            <span className="unread">•</span>
                          )}
                      </div>
                    </div>
                  </button>
                );
              })}
              {!visible.length &&
                empty(
                  t(!demo && syncGuard.current.required && !syncGuard.current.ready ? "syncPreparing" : query ? "noResults" : "emptyChats"),
                  <MessageCircle />,
                  () => void (demo ? bootstrap() : sync()),
                )}
            </>
          )}
        </div>
        <footer className="sidebar-footer">
          <ShieldCheck size={15} />
          <span>{t(demo ? "localOnly" : online ? "secure" : "offline")}</span>
          <span className="footer-line" />
          <button onClick={() => setModal("connect")}>{t("connect")}</button>
        </footer>
      </aside>
      <main id="conversation" className="workspace">
        {section === "settings" ? (
          <>
            <header className="thread-header">
              <IconButton
                label={t("back")}
                onClick={() => {
                  setSection("messages");
                  setMobileOpen(false);
                }}
              >
                <ArrowLeft />
              </IconButton>
              <h2>{t("settings")}</h2>
            </header>
            <div className="settings-content">
              <div className="profile-card">
                <Avatar
                  name={me?.displayName ?? "Jobs"}
                  src={me?.avatarThumbnailURL || me?.avatarURL}
                  size={64}
                />
                <div>
                  <h2>{me?.displayName ?? "JobsIM"}</h2>
                  <p>{demo ? t("demo") : "@" + me?.username}</p>
                </div>
              </div>
              <section className="settings-section">
                <h3>{t("theme")}</h3>
                <div className="segmented">
                  {[
                    ["light", Sun],
                    ["dark", Moon],
                    ["system", Monitor],
                  ].map(([key, Icon]) => (
                    <button
                      key={String(key)}
                      aria-pressed={theme === key}
                      className={theme === key ? "chosen" : ""}
                      onClick={() => setTheme(String(key))}
                    >
                      <Icon size={18} />
                      {t(String(key))}
                    </button>
                  ))}
                </div>
                <label className="setting-line">
                  <span>{t("language")}</span>
                  <select
                    value={locale}
                    onChange={(e) => setLocale(e.target.value)}
                  >
                    {Object.entries(languages).map(([key, name]) => (
                      <option key={key} value={key}>
                        {name}
                      </option>
                    ))}
                  </select>
                </label>
                <label className="setting-line">
                  <span>{t("font")}</span>
                  <select
                    value={effectiveFont}
                    disabled={selfBusy}
                    onChange={(e) => void changeFont(e.target.value).catch(report)}
                  >
                    {["small", "normal", "large"].map((f) => (
                      <option key={f} value={f}>
                        {t(f)}
                      </option>
                    ))}
                  </select>
                </label>
                {!demo && features.sync_self_state === true && <p className="self-state-hint">{selfLabel(locale, "cloudFont")}</p>}
                {!!fontPending.length && !demo && features.sync_self_state === true && <button className="secondary" disabled={selfBusy} onClick={() => void retryFont().catch(report)}>{selfLabel(locale, "retryOriginal")}</button>}
              </section>
              <section className="settings-section">
                <h3>{t("privacy")}</h3>
                <button
                  className="setting-line"
                  onClick={() => setModal("connect")}
                >
                  <span>
                    <Server size={18} />
                    {t("server")}
                  </span>
                  <span>
                    {location.host}
                    <ChevronRight size={17} />
                  </span>
                </button>
                <button
                  className="setting-line"
                  onClick={() => {
                    setModal("devices");
                    if (!demo)
                      void api<{ items: any[] }>("/auth/sessions")
                        .then((r) => setSessions(r.items ?? []))
                        .catch(report);
                  }}
                >
                  <span>
                    <Monitor size={18} />
                    {t("devices")}
                  </span>
                  <ChevronRight size={17} />
                </button>
                <button
                  className="setting-line"
                  onClick={() => setModal("codes")}
                >
                  <span>
                    <Lock size={18} />
                    {t("generateCodes")}
                  </span>
                  <ChevronRight size={17} />
                </button>
                <button
                  className="setting-line"
                  onClick={() => {
                    if (notify) {
                      setNotify(false);
                      return;
                    }
                    if ("Notification" in window)
                      void Notification.requestPermission().then(
                        (permission) => {
                          setNotify(permission === "granted");
                          setNotice(
                            t(
                              permission === "granted"
                                ? "saved"
                                : "notSupported",
                            ),
                          );
                        },
                      );
                    else setNotice(t("notSupported"));
                  }}
                >
                  <span>
                    <Globe size={18} />
                    {t("notifications")}
                  </span>
                  <span>
                    {t(notify ? "on" : "off")}
                    <ChevronRight size={17} />
                  </span>
                </button>
              </section>
              {!demo && session && features.sync_self_state === true && features.cloud_drafts === true && syncGuard.current.ready && <SelfDraftRecovery
                key={namespace.current + "|draft-recovery|" + authSnapshot().auth?.familyId} actions={captureSelfActions()}
                conversationIds={space.conversations.map(row => row.id)} locale={locale} t={t}
                onChanged={() => void sync()} onError={report}
              />}
              <DataExportPanel key={namespace.current + "|data-export|" + authSnapshot().auth?.familyId}
                context={captureDataExportContext()} enabled={!demo && features.chat_export === true} locale={locale} t={t}
                onConnect={() => setModal("connect")} onReload={() => void sync()} />
              <MigrationEntry context={captureGroupContext()} enabled={!demo && features.authoritative_sync === true && syncGuard.current.ready}
                observations={JSON.stringify([space.conversations.map(row => [row.id, row.membershipGeneration, row.state, row.readOnly]), Object.values(space.messages).flat().filter(row => row.forwarded || row.deleted).map(row => [row.id, row.contentVersion, row.deleted, row.contentUnavailable, row.forwardUnavailable])])}
                administrator={session?.user.admin === true} locale={locale} t={t} onLogin={() => setModal("signIn")} onReload={() => void sync()} />
              <ReceiptEntry context={captureGroupContext()} enabled={!demo && features.authoritative_sync === true && syncGuard.current.ready}
                locale={locale} t={t} onLogin={() => setModal("signIn")} />
              <PasswordChangePanel context={capturePasswordChangeContext()} enabled={!demo && features.own_password_change === true}
                administrator={session?.user.admin === true} locale={locale} t={t} onLogin={() => setModal("signIn")} />
              <AccountDeletionPanel context={captureAccountDeletionContext()} enabled={!demo && features.own_account_deletion === true}
                administrator={session?.user.admin === true} locale={locale} t={t} onLogin={() => setModal("signIn")} />
              {!demo && features.reports && (
                <button
                  className="setting-line"
                  onClick={() => {
                    setReportTarget(null);
                    setModal("reports");
                  }}
                >
                  <span>
                    <ShieldCheck size={18} />
                    {t("myReports")}
                  </span>
                  <ChevronRight size={17} />
                </button>
              )}
              {!demo && session?.user.admin && (
                <button
                  className="setting-line"
                  onClick={() => setModal("admin")}
                >
                  <span>
                    <ShieldCheck size={18} />
                    {t("admin")}
                  </span>
                  <ChevronRight size={17} />
                </button>
              )}
              <button
                className="secondary"
                onClick={() => {
                  if (demo) setModal("signIn");
                  else void signOut();
                }}
              >
                <LogOut size={17} />
                {t(demo ? "signIn" : "logout")}
              </button>
              <p className="settings-footnote">
                JobsIM · {t(demo ? "localOnly" : online ? "secure" : "offline")}
              </p>
            </div>
          </>
        ) : conversation ? (
          <>
            <header className="thread-header">
              <div className="mobile-back">
                <IconButton
                  label={t("back")}
                  onClick={() => setMobileOpen(false)}
                >
                  <ArrowLeft size={21} />
                </IconButton>
              </div>
              <Avatar
                name={conversationLabel(conversation)}
                src={conversationAvatar(conversation)}
                group={conversation.kind === "group"}
                size={40}
              />
              <button
                className="thread-title"
                onClick={() => void showMembers()}
              >
                <h2>{conversationLabel(conversation)}</h2>
                <span>
                  {conversation.kind === "group"
                    ? conversation.memberIds.length + " " + t("members")
                    : t(conversation.kind === "self" ? "note" : "direct")}
                </span>
              </button>
              <div className="header-actions">
                <IconButton
                  label={t("search")}
                  onClick={() => setModal("search")}
                >
                  <Search size={20} />
                </IconButton>
                <IconButton
                  label={t("info")}
                  active={details}
                  onClick={() => void showMembers()}
                >
                  <MoreHorizontal size={23} />
                </IconButton>
              </div>
            </header>
            {demo ? (
              <div className="notice-banner demo-banner">
                <span>
                  <span className="demo-tag">{t("demo")}</span>
                  {t("demoHint")}
                </span>
                <button
                  onClick={() => {
                    if (session) {
                      setDemo(false);
                      setSelected(live.conversations[0]?.id ?? "");
                    } else setModal("signIn");
                  }}
                >
                  {t(session ? "returnLive" : "signIn")}
                  <ChevronRight size={14} />
                </button>
              </div>
            ) : !online ? (
              <div className="notice-banner offline-banner">
                <span>
                  <WifiOff size={16} />
                  {t("offline")} · {t("unknownResult")}
                </span>
                <button onClick={() => void retryOutbox().then(sync)}>
                  {t("retry")}
                </button>
              </div>
            ) : null}
            <div
              className="thread-body"
              ref={thread}
              onScroll={(event) => {
                const box = event.currentTarget;
                readingBottom.current =
                  box.scrollHeight - box.scrollTop - box.clientHeight < 100;
                if (readingBottom.current) markRead();
                if (!demo && features.sync_self_state === true) setVisibleRevision(value => value + 1);
              }}
            >
              {messages.some((m) => m.pinned && !m.deleted) && (
                <div className="pinned-bar">
                  <Pin size={16} />
                  <span>
                    {messageText(messages.find((m) => m.pinned && !m.deleted))}
                  </span>
                </div>
              )}
              {messages.map((m, i) => {
                const own = m.senderId === me?.id;
                const prev = messages[i - 1];
                const grouped =
                  prev?.senderId === m.senderId &&
                  dayKey(prev.createdAt) === dayKey(m.createdAt) &&
                  new Date(m.createdAt).getTime() -
                    new Date(prev.createdAt).getTime() <
                    180000;
                const attachment = attachmentPath(m);
                return (
                  <Fragment key={m.id}>
                    {(!prev ||
                      dayKey(prev.createdAt) !== dayKey(m.createdAt)) && (
                      <div className="date-divider">
                        <span>
                          {new Intl.DateTimeFormat(locale, {
                            year: "numeric",
                            month: "long",
                            day: "numeric",
                          }).format(new Date(m.createdAt))}
                        </span>
                      </div>
                    )}
                    <article
                      className={
                        "message " +
                        (own ? "own" : "") +
                        " " +
                        (grouped ? "grouped" : "")
                      }
                      key={m.id}
                      id={"message-" + m.id}
                      tabIndex={0}
                      aria-label={
                        displayUser(m.senderId) +
                        ": " +
                        (m.deleted
                          ? t("recall")
                          : m.contentUnavailable || m.forwardUnavailable
                            ? t("contentUnavailable")
                            : m.text)
                      }
                    >
                      {!own && !grouped ? (
                        <Avatar
                          name={displayUser(m.senderId)}
                          src={avatarFor(m.senderId)}
                          size={34}
                        />
                      ) : (
                        <span className="avatar-spacer" />
                      )}
                      <div className="message-content">
                        {!own && !grouped && (
                          <span className="sender-name">
                            {displayUser(m.senderId)}
                          </span>
                        )}
                        <div
                          className={"bubble " + (m.deleted ? "recalled" : "")}
                        >
                          {m.replyToId &&
                            !(m.contentUnavailable || m.forwardUnavailable) && (
                              <div className="reply-preview">
                                <Reply size={13} />
                                {messageText(
                                  space.messages[selected]?.find(
                                    (x) => x.id === m.replyToId,
                                  ),
                                ) || t("reply")}
                              </div>
                            )}
                          {m.deleted ? (
                            <span>{t("recall")}</span>
                          ) : m.contentUnavailable || m.forwardUnavailable ? (
                            <>
                              <span>{t("contentUnavailable")}</span>
                              {m.forwarded && !m.pending && (
                                <button className="secondary" onClick={() => void recheckForward(m)}>
                                  <RefreshCw size={13} /> {t("reload")}
                                </button>
                              )}
                            </>
                          ) : (
                            <>
                              {attachment ? (
                                <PrivateMedia
                                  path={attachment}
                                  type={m.type}
                                  name={m.text}
                                  version={m.version}
                                  t={t}
                                  playback={{ namespace: demo ? "" : namespace.current, messageId: m.id, conversationId: m.conversationId, contentVersion: m.contentVersion ?? m.version, familyId: session?.familyId, forwarded: m.forwarded, locale }}
                                />
                              ) : (
                                <p>{messageText(m)}</p>
                              )}
                              {m.forwarded && (
                                <small className="mention-label">
                                  {t("forwarded")}
                                </small>
                              )}
                              {m.mentionedMe && (
                                <small className="mention-badge">
                                  {t("mentionedYou")}
                                </small>
                              )}
                              {(m.mentionAll ||
                                !!m.mentionedUserIds?.length) && (
                                <small className="mention-label">
                                  {m.mentionAll
                                    ? t("mentionAll")
                                    : (m.mentionedUserIds ?? [])
                                        .map((id) => "@" + displayUser(id))
                                        .join(" ")}
                                </small>
                              )}
                              {!m.deleted &&
                                !m.forwarded &&
                                m.type !== "file" &&
                                m.type !== "image" &&
                                (m.contentVersion ?? m.version) !== "1" && (
                                  <small className="edited-mark">
                                    {t("edit")}
                                  </small>
                                )}
                            </>
                          )}
                        </div>
                        <div className="message-meta">
                          <time>{date(m.createdAt)}</time>
                          {m.pinned && <Pin size={10} />}{" "}
                          {m.favorite && <Bookmark size={10} />}{" "}
                          {own &&
                            (m.pending ? (
                              <button onClick={() => void writeMessage(m)}>
                                {t("pending")}
                              </button>
                            ) : (
                              <Check size={12} aria-label={t("accepted")} />
                            ))}
                        </div>
                        {!(m.contentUnavailable || m.forwardUnavailable) &&
                          !!m.reactions?.length && (
                            <button
                              className="reaction-chip"
                              onClick={() => void mutate(m, "reaction")}
                            >
                              {m.reactions
                                .map((r: any) =>
                                  typeof r === "string" ? r : r.emoji,
                                )
                                .join(" ")}{" "}
                              {m.reactions.length}
                            </button>
                          )}
                      </div>
                      {(m.contentUnavailable || m.forwardUnavailable) &&
                        !m.deleted &&
                        own && (
                          <button
                            className="text-danger"
                            onClick={() => void mutate(m, "recall")}
                          >
                            {t("recall")}
                          </button>
                        )}
                      {!m.deleted &&
                        !(m.contentUnavailable || m.forwardUnavailable) && (
                          <div className="message-actions">
                            <IconButton
                              label={t("reply")}
                              onClick={() => void mutate(m, "reply")}
                            >
                              <Reply size={15} />
                            </IconButton>
                            <IconButton
                              label={t("favorite")}
                              onClick={() => void mutate(m, "favorite")}
                            >
                              <Bookmark size={15} />
                            </IconButton>
                            <IconButton
                              label={t("manage")}
                              active={more === m.id}
                              onClick={() => setMore(more === m.id ? "" : m.id)}
                            >
                              <MoreHorizontal size={17} />
                            </IconButton>
                            {more === m.id && (
                              <div className="message-menu">
                                {[
                                  ...(!demo && features.reports
                                    ? [[t("report"), "report", ShieldCheck]]
                                    : []),
                                  [t("copy"), "copy", Copy],
                                  ...(!demo &&
                                  features.announcements &&
                                  conversation.kind === "group" &&
                                  conversation.ownerId === me?.id &&
                                  !conversation.readOnly
                                    ? [[t("setAnnouncement"), "announce", Pin]]
                                    : []),
                                  [t(m.pinned ? "unpin" : "pin"), "pin", Pin],
                                  [t("forward"), "forward", CornerUpRight],
                                  [t("react"), "reaction", Smile],
                                  ...(own && !m.forwarded
                                    ? [[t("edit"), "edit-start", Edit3]]
                                    : []),
                                  ...(own
                                    ? [[t("recall"), "recall", Trash2]]
                                    : []),
                                ].map(([label, action, Icon]) => (
                                  <button
                                    key={String(action)}
                                    onClick={() =>
                                      void mutate(m, String(action))
                                    }
                                  >
                                    <Icon size={15} />
                                    {String(label)}
                                  </button>
                                ))}
                              </div>
                            )}
                          </div>
                        )}
                    </article>
                  </Fragment>
                );
              })}
              {!messages.length &&
                empty(t("emptyMessages"), <MessageCircle size={26} />)}
              <div className="thread-spacer" />
            </div>
            {conversation.readOnly && (
              <div className="notice-banner">{t("readOnlyConversation")}</div>
            )}
            {!conversation.readOnly && !canSend && (
              <div className="notice-banner">
                {t("sendPermissionUnavailable")}
              </div>
            )}
            <form className="composer" onSubmit={(e) => void send(e)}>
              {(reply || editing) && (
                <div className="composer-reply">
                  <Reply size={15} />
                  <div>
                    <strong>{t(editing ? "edit" : "reply")}</strong>
                    <span>{messageText(reply ?? editing ?? undefined)}</span>
                  </div>
                  <IconButton
                    label={t("cancel")}
                    onClick={() => {
                      localDraftRevision.current++;
                      setReply(null);
                      setEditing(null);
                    }}
                  >
                    <X size={16} />
                  </IconButton>
                </div>
              )}
              {(mentionDraft.all || !!mentionDraft.userIds.length) && (
                <div className="composer-reply">
                  <span>
                    {mentionDraft.all
                      ? t("mentionAll")
                      : mentionDraft.userIds
                          .map((id) => "@" + displayUser(id))
                          .join(" ")}
                  </span>
                  <IconButton
                    label={t("cancel")}
                    onClick={() => {
                      localDraftRevision.current++;
                      setSpace((value) => ({
                        ...value,
                        mentionDrafts: {
                          ...value.mentionDrafts,
                          [selected]: { userIds: [], all: false },
                        },
                      }));
                    }}
                  >
                    <X size={15} />
                  </IconButton>
                </div>
              )}
              <div className="composer-accessories">
              <div className="composer-tools">
                <IconButton
                  label={t("attachment")}
                  onClick={() => fileInput.current?.click()}
                  disabled={
                    busy || !canSend || (!demo && features.media !== true)
                  }
                >
                  <Paperclip size={20} />
                </IconButton>
                <IconButton
                  label={t("emoji")}
                  onClick={() => setModal("emoji")}
                >
                  <Smile size={20} />
                </IconButton>
                {conversation.kind === "group" &&
                  (demo || features.mentions === true) && (
                    <IconButton
                      label={t("mentions")}
                      active={mentionDraft.all || !!mentionDraft.userIds.length}
                      disabled={busy || !canSend}
                      onClick={() => {
                        setModal("mentions");
                        void refreshMembers();
                      }}
                    >
                      <span aria-hidden="true">@</span>
                    </IconButton>
                  )}
                <span className="composer-context">
                  {t(conversation.kind === "group" ? "group" : "direct")}
                </span>
              </div>
              <AudioRecorderPanel contextKey={[demo ? "demo" : namespace.current, session?.familyId ?? "", selected, reply?.id ?? "", editing?.id ?? ""].join("|")}
                disabled={busy || !canSend || !!editing || (!demo && features.media !== true)} locale={locale} onSend={upload} />
              </div>
              <AttachmentSendPanel context={demo ? undefined : captureAttachmentContext()} revision={attachmentRevision} locale={locale}
                conversationName={id => { const source = space.conversations.find(value => value.id === id); return source ? conversationLabel(source) : id; }} />
              <div className="composer-draft">
              <textarea
                ref={composer}
                name="message"
                aria-label={t("placeholder")}
                placeholder={t("placeholder")}
                value={draft}
                onChange={(e) => updateDraft(e.target.value)}
                onKeyDown={(e) => {
                  if (
                    e.key === "Enter" &&
                    !e.shiftKey &&
                    !e.nativeEvent.isComposing
                  ) {
                    e.preventDefault();
                    void send();
                  }
                }}
                rows={2}
              />
              <div className="composer-bottom">
                <span>
                  <Lock size={12} />
                  {demo ? t("localOnly") : t("secure")}
                </span>
                <button
                  className="send-button"
                  type="submit"
                  disabled={busy || !canSend || !draft.trim()}
                >
                  {busy ? t("working") : t("send")}
                  <Send size={16} />
                </button>
              </div>
              </div>
              <input
                ref={fileInput}
                className="sr-only"
                type="file"
                aria-label={t("attachment")}
                onChange={(e) => {
                  const f = e.target.files?.[0];
                  if (f) void upload(f);
                  e.target.value = "";
                }}
              />
            </form>
          </>
        ) : (
          <>
            <header className="thread-header">
              <h2>{t("messages")}</h2>
              <div className="header-actions">
                <button
                  className="secondary"
                  onClick={() => setModal("newChat")}
                >
                  {t("newChat")}
                </button>
              </div>
            </header>
            {empty(t("noSelection"), <MessageCircle size={32} />, () =>
              setModal("newChat"),
            )}
          </>
        )}
      </main>
      {details && conversation && (
        <aside className="detail-panel">
          <header>
            <h2>{t("info")}</h2>
            <IconButton label={t("close")} onClick={() => setDetails(false)}>
              <X size={19} />
            </IconButton>
          </header>
          <div className="detail-identity">
            <Avatar
              name={conversationLabel(conversation)}
              src={conversationAvatar(conversation)}
              size={72}
              group={conversation.kind === "group"}
            />
            <h3>{conversationLabel(conversation)}</h3>
            <p>{t(conversation.kind === "group" ? "group" : "direct")}</p>
          </div>
          {!demo && session && features.sync_self_state === true && syncGuard.current.ready && <SelfStatePanel
            key={namespace.current + "|" + conversation.id + "|self-state"}
            actions={captureSelfActions()} conversationId={conversation.id} locale={locale}
            localDraft={draft} selectedReply={reply} cloudEnabled={features.cloud_drafts === true}
            t={t} onAdopt={adoptCloudText} onChanged={() => void sync()} onError={report}
          />}
          {conversation.kind === "group" ? (
            <GroupControls
              conversation={conversation}
              members={members}
              actorId={me?.id ?? ""}
              busy={busy}
              t={t}
              avatar={(user) => (
                <Avatar
                  name={user.displayName}
                  src={user.avatarThumbnailURL || user.avatarURL}
                  size={34}
                />
              )}
              onReload={() => void refreshMembers()}
              onAction={groupAction}
              hasMore={!!memberCursor}
              onLoadMore={() => void refreshMembers(conversation.id, true)}
              governance={!demo ? <GroupGovernancePanel context={captureGroupContext()} conversationId={conversation.id} members={members}
                locale={locale} t={t} onMessage={setNotice} onChanged={async () => { await sync(); await refreshMembers(conversation.id); }} /> : undefined}
            />
          ) : (
            <>
              <h3 className="detail-label">
                {t("members")} <span>{members.length}</span>
              </h3>
              {members.map((user) => (
                <div className="member-row" key={user.id}>
                  <Avatar
                    name={user.displayName}
                    src={user.avatarThumbnailURL || user.avatarURL}
                    size={34}
                  />
                  <span>{user.displayName}</span>
                </div>
              ))}
            </>
          )}
          {conversation.kind === "group" &&
            !demo &&
            features.announcements &&
            session && (
              <AnnouncementPanel
                key={
                  namespace.current + "|" + conversation.id + "|announcement"
                }
                conversation={conversation}
                namespace={namespace.current}
                restoreEpoch={session.restoreEpoch ?? "1"}
                t={t}
                onError={report}
                onNotice={setNotice}
                onChoose={chooseLookup}
              />
            )}
          {conversation.kind === "group" &&
            conversation.ownerId === me?.id &&
            !conversation.readOnly && (
              <button
                className="secondary"
                disabled={busy}
                onClick={() => setModal("addMember")}
              >
                <Plus size={16} />
                {t("add")}
              </button>
            )}
          {conversation.kind === "direct" &&
            !demo &&
            conversation.memberIds.some((id) => id !== me?.id) && (
              <PeerControls
                key={
                  namespace.current +
                  "|" +
                  conversation.id +
                  "|" +
                  session?.restoreEpoch
                }
                namespace={namespace.current}
                restoreEpoch={session?.restoreEpoch ?? "1"}
                peerId={conversation.memberIds.find((id) => id !== me?.id)!}
                t={t}
                onNotice={setNotice}
                onError={report}
                onBoundary={() => freezeDirectQueue(conversation.id)}
                onChanged={() => {
                  void refreshContacts();
                  void sync();
                }}
              />
            )}
          {!demo && features.reports && conversation.kind !== "self" && (
            <button
              className="secondary"
              onClick={() => {
                const peer = conversation.memberIds.find((id) => id !== me?.id);
                setReportTarget(
                  conversation.kind === "direct" && peer
                    ? {
                        targetType: "user",
                        targetId: peer,
                        label: displayUser(peer),
                      }
                    : {
                        targetType: "conversation",
                        targetId: conversation.id,
                        label: conversationLabel(conversation),
                      },
                );
                setModal("reports");
              }}
            >
              <ShieldCheck size={16} />
              {t("report")}
            </button>
          )}
          <div className="detail-footer">
            <ShieldCheck size={17} />
            {t("privacy")}
          </div>
        </aside>
      )}
      {modal && (
        <div
          className="modal-overlay"
          onMouseDown={(e) => {
            if (e.target === e.currentTarget) closeModal();
          }}
        >
          <section
            className={
              "modal " +
              (modal === "admin" ? "admin-modal " : "") +
              (["signIn", "register", "recovery"].includes(modal)
                ? "auth-modal"
                : "")
            }
            role="dialog"
            aria-modal="true"
            aria-labelledby="modal-title"
            onKeyDown={(e) => {
              if (e.key === "Escape") closeModal();
              if (e.key === "Tab") {
                const items = [
                  ...e.currentTarget.querySelectorAll<HTMLElement>(
                    "button:not(:disabled),input,select,textarea,a[href]",
                  ),
                ];
                const first = items[0],
                  last = items.at(-1);
                if (e.shiftKey && document.activeElement === first) {
                  e.preventDefault();
                  last?.focus();
                } else if (!e.shiftKey && document.activeElement === last) {
                  e.preventDefault();
                  first?.focus();
                }
              }
            }}
          >
            <IconButton label={t("close")} onClick={closeModal}>
              <X size={20} />
            </IconButton>
            {["signIn", "register", "recovery"].includes(modal) ? (
              <>
                <img src="/Logo.svg" width="44" height="44" alt="JobsIM" />
                <h2 id="modal-title">{t(modal)}</h2>
                <p>{t("loginHint")}</p>
                <form
                  onSubmit={(e) => void auth(e)}
                  onChange={() => {
                    authRequest.current = "";
                  }}
                >
                  <label>
                    {t("username")}
                    <input
                      required
                      name="username"
                      autoComplete="username"
                      defaultValue={pendingInitial()?.username ?? authSnapshot().auth?.username ?? ""}
                      minLength={3}
                      maxLength={32}
                      pattern="[A-Za-z0-9_]+"
                      spellCheck={false}
                    />
                  </label>
                  {modal === "register" && (
                    <label>
                      {t("displayName")}
                      <input
                        name="displayName"
                        autoComplete="nickname"
                        maxLength={60}
                      />
                    </label>
                  )}
                  {modal === "recovery" && (
                    <label>
                      {t("recoveryCode")}
                      <input
                        required
                        name="code"
                        autoComplete="one-time-code"
                      />
                    </label>
                  )}
                  <label>
                    {t(modal === "recovery" ? "newPassword" : "password")}
                    <input
                      required
                      type="password"
                      name="password"
                      autoComplete={
                        modal === "signIn" ? "current-password" : "new-password"
                      }
                      minLength={15}
                      maxLength={128}
                    />
                  </label>
                  {requiresOTP && (
                    <label>
                      {t("otp")}
                      <input
                        name="otp"
                        required
                        inputMode="numeric"
                        autoComplete="one-time-code"
                        pattern="[0-9]{6}"
                        minLength={6}
                        maxLength={6}
                      />
                    </label>
                  )}
                  <small>{t("passwordHint")}</small>
                  <button className="primary" disabled={busy}>
                    {t(busy ? "working" : modal)}
                  </button>
                  {modal !== "recovery" && features.auth_retirement_chain === true && pendingInitial() && (
                    <button className="secondary" type="submit" value="fresh-proof" disabled={busy}>
                      {t("authFreshProof")}
                    </button>
                  )}
                </form>
                <div className="auth-links">
                  {["signIn", "register", "recovery"]
                    .filter((m) => m !== modal)
                    .map((m) => (
                      <button key={m} onClick={() => setModal(m)}>
                        {t(m)}
                      </button>
                    ))}
                  <button onClick={() => setModal("accountDeletion")}>{accountDeletionLabel(locale, "title")}</button>
                </div>
              </>
            ) : modal === "accountDeletion" ? (
              <>
                <h2 id="modal-title">{accountDeletionLabel(locale, "title")}</h2>
                <AccountDeletionPanel context={captureAccountDeletionContext()} enabled={!demo && features.own_account_deletion === true}
                  administrator={session?.user.admin === true} locale={locale} t={t} onLogin={() => setModal("signIn")} />
              </>
            ) : modal === "joinGroup" ? (
              <>
                <h2 id="modal-title">{groupLabel(locale, "joinByCode")}</h2>
                <GroupJoinPanel context={captureGroupContext()} locale={locale} t={t} onChanged={async () => { await sync(); }} onLogin={() => setModal("signIn")} />
              </>
            ) : modal === "connect" ? (
              <>
                <Server size={30} />
                <h2 id="modal-title">{t("connect")}</h2>
                <p>{t("serverHint")}</p>
                <form
                  onSubmit={(e) => {
                    e.preventDefault();
                    const raw = String(
                      new FormData(e.currentTarget).get("server") ?? "",
                    );
                    try {
                      const url = new URL(raw);
                      if (
                        url.protocol !== "https:" ||
                        url.username ||
                        url.password ||
                        url.search ||
                        url.hash
                      )
                        throw Error();
                      if (url.origin === location.origin) {
                        void bootstrap();
                        setModal("");
                      } else location.assign(url.origin);
                    } catch {
                      setNotice(t("connectionError"));
                    }
                  }}
                >
                  <label>
                    {t("server")}
                    <input
                      type="url"
                      name="server"
                      defaultValue={location.origin}
                      required
                      autoComplete="url"
                    />
                  </label>
                  <button className="primary">{t("connect")}</button>
                </form>
                <div className="server-status">
                  <ShieldCheck size={17} />
                  {t("connectedTo")}: {location.host}
                </div>
              </>
            ) : modal === "newChat" || modal === "addMember" ? (
              <>
                <h2 id="modal-title">
                  {t(modal === "addMember" ? "members" : "newChat")}
                </h2>
                {modal === "newChat" && <button type="button" className="secondary" onClick={() => setModal("joinGroup")}>{groupLabel(locale,"joinByCode")}</button>}
                <label className="search">
                  <Search size={17} />
                  <input
                    autoFocus
                    placeholder={t("searchPeople")}
                    aria-label={t("searchPeople")}
                    onChange={(e) => void findPeople(e.target.value)}
                  />
                </label>
                <div className="people-list">
                  {peopleQueried &&
                    !people.length &&
                    empty(t("noResults"), <Users />, () => void findPeople(""))}
                  {(peopleQueried ? people : contacts).map((u) => (
                    <div className="person-choice" key={u.id}>
                      <button
                        onClick={() => {
                          if (modal === "addMember") {
                            if (demo)
                              setSample((s) => ({
                                ...s,
                                conversations: s.conversations.map((c) =>
                                  c.id === selected
                                    ? {
                                        ...c,
                                        memberIds: [
                                          ...new Set([...c.memberIds, u.id]),
                                        ],
                                      }
                                    : c,
                                ),
                              }));
                            else
                              void api(
                                "/conversations/" + selected + "/members",
                                json("POST", { userId: u.id }),
                              )
                                .then(() => {
                                  setNotice(t("saved"));
                                  closeModal();
                                  void refreshMembers();
                                })
                                .catch(report);
                          } else void createConversation([u.id]);
                        }}
                      >
                        <Avatar
                          name={u.displayName}
                          src={u.avatarThumbnailURL || u.avatarURL}
                          size={38}
                        />
                        <span>
                          {u.displayName}
                          <small>@{u.username}</small>
                        </span>
                      </button>
                      <input
                        type="checkbox"
                        aria-label={t("chooseMembers") + " " + u.displayName}
                        checked={pick.includes(u.id)}
                        onChange={(e) =>
                          setPick((v) =>
                            e.target.checked
                              ? [...v, u.id]
                              : v.filter((x) => x !== u.id),
                          )
                        }
                      />
                      <IconButton
                        label={t("add")}
                        onClick={() => {
                          if (demo) {
                            setNotice(t("requestSent"));
                            return;
                          }
                          void api(
                            "/contacts/requests",
                            json("POST", { userId: u.id, requestId: uid() }),
                          )
                            .then(() => setNotice(t("requestSent")))
                            .catch(report);
                        }}
                      >
                        <Plus size={17} />
                      </IconButton>
                    </div>
                  ))}
                </div>
                {modal === "newChat" && (
                  <form
                    onSubmit={(e) => {
                      e.preventDefault();
                      void createConversation(
                        pick,
                        "group",
                        String(
                          new FormData(e.currentTarget).get("title") ?? "",
                        ),
                      );
                    }}
                  >
                    <label>
                      {t("title")}
                      <input name="title" required placeholder={t("group")} />
                    </label>
                    <button className="primary" disabled={!pick.length}>
                      {t("createGroup")}
                    </button>
                  </form>
                )}
              </>
            ) : modal === "forward" ? (
              <>
                <h2 id="modal-title">{t("forward")}</h2>
                {forwarding && <p>{messageText(forwarding)}</p>}
                {space.conversations.map((c) => (
                  <button
                    key={c.id}
                    className="contact-row"
                    disabled={busy || c.readOnly || (!demo && !c.sendAdmission)}
                    onClick={() => void forwardTo(c)}
                  >
                    <Avatar
                      name={conversationLabel(c)}
                      src={conversationAvatar(c)}
                      size={36}
                    />
                    {conversationLabel(c)}
                    <ChevronRight size={17} />
                  </button>
                ))}
                {!space.conversations.length &&
                  empty(t("emptyChats"), <MessageCircle />)}
              </>
            ) : modal === "search" ? (
              <>
                <h2 id="modal-title">{t("search")}</h2>
                <MessageLookup
                  key={
                    String(demo) +
                    "|" +
                    namespace.current +
                    "|search|" +
                    selected
                  }
                  kind="search"
                  demo={demo}
                  cached={Object.values(space.messages).flat()}
                  conversationId={selected || undefined}
                  t={t}
                  text={messageText}
                  date={date}
                  onChoose={chooseLookup}
                  onError={report}
                />
              </>
            ) : modal === "mentions" && conversation ? (
              <MentionPicker
                members={members}
                actorId={me?.id ?? ""}
                owner={
                  conversation.ownerId === me?.id &&
                  (demo || features.role_admission === true)
                }
                value={mentionDraft}
                t={t}
                onReload={() => void refreshMembers()}
                onChange={(value) => {
                  localDraftRevision.current++;
                  setSpace((space) => ({
                    ...space,
                    mentionDrafts: {
                      ...space.mentionDrafts,
                      [selected]: value,
                    },
                  }));
                }}
              />
            ) : modal === "announcement" && conversation && session && !demo ? (
              <>
                <h2 id="modal-title">{t("announcement")}</h2>
                <AnnouncementPanel
                  key={
                    namespace.current +
                    "|" +
                    conversation.id +
                    "|announcement-modal"
                  }
                  conversation={conversation}
                  namespace={namespace.current}
                  restoreEpoch={session.restoreEpoch ?? "1"}
                  source={announcementSource}
                  t={t}
                  onError={report}
                  onNotice={setNotice}
                  onChoose={chooseLookup}
                />
              </>
            ) : modal === "reports" && session && !demo ? (
              <ReportPanel
                key={
                  namespace.current + "|" + (reportTarget?.targetId ?? "list")
                }
                namespace={namespace.current}
                restoreEpoch={session.restoreEpoch ?? "1"}
                target={reportTarget}
                t={t}
                onNotice={setNotice}
              />
            ) : modal === "admin" && session?.user.admin && !demo ? (
              <AdminPanel
                t={t}
                actorId={session.user.id}
                canWrite={features.admin_writes === true}
                reportsEnabled={features.reports === true}
                onNotice={setNotice}
                onFeatures={publishFeatures}
              />
            ) : modal === "emoji" ? (
              <>
                <h2 id="modal-title">{t("emoji")}</h2>
                <div className="emoji-grid">
                  {[
                    "😊",
                    "👍",
                    "❤️",
                    "🎉",
                    "🙏",
                    "😂",
                    "😮",
                    "👏",
                    "✅",
                    "🤝",
                    "🌹",
                    "💡",
                  ].map((emoji) => (
                    <button
                      key={emoji}
                      aria-label={t("emoji") + " " + emoji}
                      onClick={() => {
                        updateDraft(draft + emoji);
                        closeModal();
                        composer.current?.focus();
                      }}
                    >
                      {emoji}
                    </button>
                  ))}
                </div>
              </>
            ) : modal === "codes" ? (
              <>
                <Lock size={28} />
                <h2 id="modal-title">{t("generateCodes")}</h2>
                <p>{t("recoveryWarning")}</p>
                {generated.length ? (
                  <pre className="recovery-codes">{generated.join("\n")}</pre>
                ) : (
                  <form
                    onSubmit={(e) => {
                      e.preventDefault();
                      if (demo) {
                        setNotice(t("notSupported"));
                        return;
                      }
                      void api<{ codes?: string[] }>(
                        "/auth/recovery-codes",
                        json("POST", {
                          password: new FormData(e.currentTarget).get(
                            "password",
                          ),
                          requestId: uid(),
                        }),
                      )
                        .then((r) => setGenerated(r.codes ?? []))
                        .catch(report);
                    }}
                  >
                    <label>
                      {t("password")}
                      <input
                        name="password"
                        type="password"
                        required
                        autoComplete="current-password"
                      />
                    </label>
                    <button className="primary">{t("generateCodes")}</button>
                  </form>
                )}
              </>
            ) : modal === "devices" ? (
              <>
                <h2 id="modal-title">{t("devices")}</h2>
                {sessions.length
                  ? sessions.map((s) => (
                      <div className="setting-line" key={s.deviceId ?? s.id}>
                        <span>
                          <Monitor size={18} />
                          {s.deviceName ?? s.name ?? s.deviceId}
                        </span>
                        <button
                          onClick={() =>
                            void api(
                              "/auth/sessions/" + (s.deviceId ?? s.id),
                              json("DELETE"),
                            )
                              .then(() =>
                                setSessions((x) => x.filter((v) => v !== s)),
                              )
                              .catch(report)
                          }
                        >
                          {t("remove")}
                        </button>
                      </div>
                    ))
                  : empty(t("devices"), <Monitor />, () => {
                      if (!demo)
                        void api<{ items: any[] }>("/auth/sessions")
                          .then((r) => setSessions(r.items))
                          .catch(report);
                    })}
              </>
            ) : (
              <>
                <Avatar
                  name={me?.displayName ?? "Jobs"}
                  src={me?.avatarThumbnailURL || me?.avatarURL}
                  size={64}
                />
                {!demo && features.avatars && (
                  <div className="avatar-actions">
                    <button
                      className="secondary"
                      onClick={() =>
                        avatarPending
                          ? void publishAvatar()
                          : avatarInput.current?.click()
                      }
                      disabled={busy}
                    >
                      {t(avatarPending ? "retry" : "changeAvatar")}
                    </button>
                    {avatarPending && (
                      <button
                        className="secondary"
                        onClick={() => void inspectAvatar(avatarPending)}
                      >
                        {t("reload")}
                      </button>
                    )}
                    <input
                      ref={avatarInput}
                      type="file"
                      accept="image/jpeg,image/png"
                      className="sr-only"
                      aria-label={t("changeAvatar")}
                      onChange={(event) => {
                        const file = event.target.files?.[0];
                        if (file) void publishAvatar(file);
                        event.target.value = "";
                      }}
                    />
                  </div>
                )}
                <h2 id="modal-title">{me?.displayName ?? "JobsIM"}</h2>
                <p>{me?.username}</p>
                <p>{t("privacy")}</p>
                <form
                  onSubmit={(e) => {
                    e.preventDefault();
                    const displayName = String(
                      new FormData(e.currentTarget).get("displayName") ?? "",
                    );
                    if (demo) {
                      setNotice(t("notSupported"));
                      return;
                    }
                    void api<User>("/me", json("PATCH", { displayName }))
                      .then((user) => {
                        setSession((s) => (s ? { ...s, user } : s));
                        setNotice(t("saved"));
                        closeModal();
                      })
                      .catch(report);
                  }}
                >
                  <label>
                    {t("displayName")}
                    <input
                      name="displayName"
                      required
                      maxLength={60}
                      defaultValue={me?.displayName}
                    />
                  </label>
                  <button className="primary">{t("save")}</button>
                </form>
              </>
            )}
          </section>
        </div>
      )}
      {notice && (
        <div className="toast" role="status" aria-live="polite">
          <span>{notice}</span>
          <button aria-label={t("close")} onClick={() => setNotice("")}>
            <X size={16} />
          </button>
        </div>
      )}
    </div>
  );
}
