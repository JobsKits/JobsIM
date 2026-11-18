package main

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

type client struct {
	base         string
	http         *http.Client
	token        string
	deployment   string
	user         string
	store        *localStore
	epoch        string
	device       string
	serverDevice string
	reader       *bufio.Reader
	lookup       *lookupSession
	auth         *authManager
	syncMu       sync.Mutex
}

func id() string {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func main() {
	if e := run(); e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(1)
	}
}
func run() error {
	dataDirectory := flag.String("data-dir", "", "Private local state root; defaults to the user configuration directory")
	base := flag.String("server", "https://127.0.0.1:8443", "JobsIM instance HTTPS URL")
	ca := flag.String("ca", "", "Instance-specific trusted CA PEM")
	dev := flag.Bool("insecure-loopback", false, "Allow HTTP on loopback for development; TLS verification is never disabled")
	flag.Parse()
	u, e := url.Parse(*base)
	if e != nil || u.Host == "" || u.Opaque != "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" {
		return fmt.Errorf("invalid server URL")
	}
	if u.Scheme != "https" {
		ip := net.ParseIP(u.Hostname())
		if !*dev || u.Scheme != "http" || ip == nil || !ip.IsLoopback() {
			return fmt.Errorf("HTTPS required; HTTP development is restricted to explicit loopback IP")
		}
	}
	roots, e := x509.SystemCertPool()
	if e != nil {
		roots = x509.NewCertPool()
	}
	if *ca != "" {
		pem, e := os.ReadFile(*ca)
		if e != nil {
			return e
		}
		if !roots.AppendCertsFromPEM(pem) {
			return fmt.Errorf("invalid CA file")
		}
	}
	c := &client{base: strings.TrimRight(*base, "/"), http: &http.Client{Timeout: 20 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{MinVersion: tls.VersionTLS12, RootCAs: roots}}, CheckRedirect: func(r *http.Request, via []*http.Request) error { return http.ErrUseLastResponse }}}
	var boot map[string]any
	if e = c.call("GET", "/api/bootstrap", nil, &boot); e != nil {
		return e
	}
	c.deployment, _ = boot["deploymentId"].(string)
	c.epoch, _ = boot["restoreEpoch"].(string)
	if c.epoch == "" {
		return fmt.Errorf("server restore epoch missing")
	}
	if c.deployment == "" {
		return fmt.Errorf("instance identity absent")
	}
	fmt.Println("JobsIM instance:", boot["name"], c.deployment)
	args := flag.Args()
	if len(args) == 0 || args[0] == "help" {
		fmt.Println("jobsim --server URL [--ca CA.pem] login|register|reauthenticate|recover|bootstrap|password-changes|password-change-reconcile ID|password-change-retry ID\nAfter login: account-delete, account-deletions, deletion-reconcile, deletion-retry, deletion-query, deletion-cancel, deletion-cancel-retry, password-change, password-changes, password-change-reconcile, password-change-retry, users, contacts, relationship, request, reopen, operations, resume, accept, reject, remove-contact, block, unblock, direct, group, members, add-member, remove-member, rename, transfer, group-policy, group-change, group-recover, group-operations, group-check, group-retry, group-invites, invite-code, create-invite, revoke-invite, join-group, group-applications, group-approve, group-reject, leave, conversations, history, sync, sync-status, sync-schema, local-history, cached, draft, drafts, font, view, read, cloud-draft, cloud-draft-set, cloud-draft-reply, cloud-draft-clear, cloud-draft-adopt, cloud-draft-recover, self-operations, resume-self, data-export, data-exports, data-export-status, data-export-reconcile, data-export-retry, data-export-cancel, data-export-download, storage-status, storage-declare, receive-text, receive-message, receive-object, receive-object-file, receipt-file-status, receipts, reconcile-receipt, retry-receipt, message, send, forward, retry, reconcile, outbox, upload, download, download-forward, edit, retract, react, pin, favorite, search, favorites, more, open-result, mention, announcement, announce, clear-announcement, announcement-operations, resume-announcement, logout, quit")
		return nil
	}
	if args[0] == "bootstrap" {
		return printJSON(boot)
	}
	if oneOf(args[0], "account-deletions", "deletion-reconcile", "deletion-retry", "deletion-query", "deletion-cancel", "deletion-cancel-retry") {
		return c.runOwnDeletionResume(*dataDirectory, *ca, args, bufio.NewReader(os.Stdin))
	}
	if oneOf(args[0], "password-change-reconcile", "password-change-retry", "password-changes") {
		return c.runPasswordChangeResume(*dataDirectory, *ca, args, bufio.NewReader(os.Stdin))
	}
	if args[0] != "login" && args[0] != "register" && args[0] != "recover" && args[0] != "reauthenticate" {
		return fmt.Errorf("start with login, register or reauthenticate; system credential protection is required by default")
	}
	reader := bufio.NewReader(os.Stdin)
	c.reader = reader
	if args[0] == "recover" {
		return c.recoverAccount()
	}
	fmt.Print("Username: ")
	username, e := reader.ReadString('\n')
	if e != nil {
		return e
	}
	root, e := os.UserConfigDir()
	if *dataDirectory != "" {
		root, e = filepath.Abs(*dataDirectory)
	}
	if e != nil {
		return e
	}
	trust := "system"
	if *ca != "" {
		data, err := os.ReadFile(*ca)
		if err != nil {
			return err
		}
		trust = fingerprint(string(data))
	}
	profile := fingerprint(c.base + ":" + trust + ":" + c.deployment)
	features, _ := boot["features"].(map[string]any)
	refresh, _ := features["auth_refresh"].(bool)
	chain, _ := features["auth_retirement_chain"].(bool)
	c.auth, e = openAuthManager(c, root, profile, username, refresh, chain, reader)
	if e != nil {
		return e
	}
	defer c.auth.close()
	restored := false
	if args[0] == "login" {
		restored, e = c.auth.restore()
		if e != nil {
			return e
		}
	}
	if !restored {
		if e = c.auth.authenticate(args[0], reader); e != nil {
			return e
		}
	}
	defer func() {
		if e := c.auth.logout(); e != nil {
			fmt.Fprintln(os.Stderr, e)
		}
	}()
	installation, e := installationIdentity(filepath.Join(root, "JobsIM", "CLI", profile))
	if e != nil {
		return e
	}
	c.device = c.serverDevice
	if e = freezePreviousCLIStores(root, c.base, profile, c.deployment, c.user, c.device, c.epoch); e != nil {
		return e
	}
	identity := localIdentity{Origin: c.base, DeploymentID: c.deployment, UserID: c.user, DeviceID: c.device, Space: "personal", ConnectionProfile: profile, StoreGeneration: installation}
	namespace, _ := json.Marshal(identity)
	directory := filepath.Join(root, "JobsIM", "CLI", profile, fingerprint(string(namespace)))
	c.store, e = openStore(filepath.Join(directory, "client.sqlite"), identity, c.epoch)
	if e != nil {
		return e
	}
	defer c.store.close()
	if e = c.auth.bindMessageStore(filepath.Join(fingerprint(string(namespace)), "client.sqlite"), installation); e != nil {
		return e
	}
	if e = c.store.drainSyncCleanup(); e != nil {
		return e
	}
	authority, _ := features["authoritative_sync"].(bool)
	responsible, responsibilityErr := c.store.syncResponsible()
	if responsibilityErr != nil {
		return responsibilityErr
	}
	if authority || responsible {
		if e = c.authoritativeSync(); e != nil {
			fmt.Fprintln(os.Stderr, "Authoritative synchronization is restricted:", e)
		}
	}
	if c.auth.temporary {
		fmt.Println("Authenticated. Explicit temporary session; credentials remain in this process and restarting requires verified authentication.")
	} else {
		fmt.Println("Authenticated. System credentials protect refresh; quit revokes this session. Unknown results require verified reauthentication.")
	}
	for {
		fmt.Print("jobsim> ")
		line, e := reader.ReadString('\n')
		if errors.Is(e, io.EOF) {
			return nil
		}
		if e != nil {
			return e
		}
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		cmd, rest, _ := strings.Cut(line, " ")
		if cmd == "quit" || cmd == "exit" {
			return nil
		}
		if e = c.command(cmd, rest); e != nil {
			fmt.Fprintln(os.Stderr, e)
		}
		if c.token == "" {
			return nil
		}
	}
}
func (c *client) callWithToken(method, path string, body any, out any, token string) error {
	var src io.Reader
	if body != nil {
		b, e := json.Marshal(body)
		if e != nil {
			return e
		}
		src = bytes.NewReader(b)
	}
	req, e := http.NewRequest(method, c.base+path, src)
	if e != nil {
		return e
	}
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, e := c.http.Do(req)
	if e != nil {
		return fmt.Errorf("result unknown: %w; preserve original message ID and reconcile", e)
	}
	defer res.Body.Close()
	data, e := readLimited(res.Body, 12<<20)
	if e != nil {
		return e
	}
	if res.StatusCode >= 300 {
		var payload struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(data, &payload)
		if payload.Error.Code == "" {
			payload.Error.Code = "INVALID_ERROR_RESPONSE"
		}
		return &apiFailure{Status: res.StatusCode, Code: payload.Error.Code, Message: payload.Error.Message}
	}
	if out != nil && len(data) > 0 {
		decoder := json.NewDecoder(bytes.NewReader(data))
		decoder.UseNumber()
		if err := decoder.Decode(out); err != nil {
			return err
		}
		var extra any
		if err := decoder.Decode(&extra); err != io.EOF {
			return fmt.Errorf("invalid trailing JSON response")
		}
		return nil
	}
	return nil
}
func printJSON(v any) error {
	b, e := json.Marshal(v)
	if e != nil {
		return e
	}
	decoder := json.NewDecoder(bytes.NewReader(b))
	decoder.UseNumber()
	var view any
	if e = decoder.Decode(&view); e != nil {
		return e
	}
	projectUnavailable(view)
	b, e = json.MarshalIndent(view, "", "  ")
	if e == nil {
		fmt.Println(string(b))
	}
	return e
}

// Presentation never exposes an older body when recovery retained only its identity.
func projectUnavailable(value any) {
	switch item := value.(type) {
	case map[string]any:
		delete(item, "forwardAdmission")
		delete(item, "replyAdmission")
		delete(item, "draftAdmission")
		if item["deleted"] == true || item["contentUnavailable"] == true || item["forwardUnavailable"] == true {
			item["text"] = ""
			item["displayText"] = "[Content unavailable after restore]"
			if item["deleted"] == true {
				item["displayText"] = "[Deleted]"
			}
			delete(item, "attachmentId")
			delete(item, "attachmentURL")
		}
		for _, child := range item {
			projectUnavailable(child)
		}
	case []any:
		for _, child := range item {
			projectUnavailable(child)
		}
	}
}
func (c *client) command(cmd, args string) error {
	if handled, err := c.ownDeletionCommand(cmd, args); handled {
		return err
	}
	if handled, err := c.groupCommand(cmd, args); handled {
		return err
	}
	if handled, err := c.passwordChangeCommand(cmd, args); handled {
		return err
	}
	if handled, err := c.dataExportCommand(cmd, args); handled {
		return err
	}
	if handled, err := c.receiptCommand(cmd, args); handled {
		return err
	}
	if handled, err := c.selfCommand(cmd, args); handled {
		return err
	}
	if handled, err := c.syncCommand(cmd, args); handled {
		return err
	}
	if handled, err := c.collaborationCommand(cmd, args); handled {
		return err
	}
	parts := strings.Fields(args)
	var result any
	method, path := "GET", ""
	var body any
	switch cmd {
	case "users":
		path = "/api/users?q=" + url.QueryEscape(args)
	case "contacts":
		path = "/api/contacts"
	case "request", "reopen":
		if len(parts) != 1 {
			return fmt.Errorf("%s USER_ID", cmd)
		}
		return c.governance(cmd, parts[0])
	case "operations":
		items, err := c.store.operations()
		if err != nil {
			return err
		}
		return printJSON(items)
	case "resume":
		if len(parts) != 1 {
			return fmt.Errorf("resume ORIGINAL_OPERATION_ID")
		}
		return c.resumeGovernance(parts[0])
	case "relationship":
		if len(parts) != 1 {
			return fmt.Errorf("relationship USER_ID")
		}
		path = "/api/contacts/" + url.PathEscape(parts[0]) + "/relationship"
	case "remove-contact":
		if len(parts) != 1 {
			return fmt.Errorf("remove-contact USER_ID")
		}
		fresh, err := c.conversationRecords()
		if err != nil {
			return err
		}
		if err = c.store.prepareRelationshipRevocation(parts[0], "CONTACT_REMOVED", fresh); err != nil {
			return err
		}
		method, path = "DELETE", "/api/contacts/"+url.PathEscape(parts[0])
	case "accept", "reject":
		if len(parts) != 1 {
			return fmt.Errorf("%s REQUEST_ID", cmd)
		}
		method = "POST"
		path = "/api/contacts/requests/" + url.PathEscape(parts[0]) + "/" + cmd
	case "direct":
		if len(parts) != 1 {
			return fmt.Errorf("direct USER_ID")
		}
		method = "POST"
		path = "/api/conversations"
		body = map[string]any{"kind": "direct", "memberIds": parts, "requestId": id()}
	case "group":
		title, members, _ := strings.Cut(args, "|")
		ids := strings.Fields(members)
		method = "POST"
		path = "/api/conversations"
		body = map[string]any{"kind": "group", "title": strings.TrimSpace(title), "memberIds": ids, "requestId": id()}
	case "conversations":
		path = "/api/conversations"
	case "members":
		if len(parts) < 1 || len(parts) > 2 {
			return fmt.Errorf("members CONVERSATION_ID [CURSOR]")
		}
		path = "/api/conversations/" + url.PathEscape(parts[0]) + "/members?limit=50"
		if len(parts) == 2 {
			path += "&after=" + url.QueryEscape(parts[1])
		}
	case "add-member":
		if len(parts) != 2 {
			return fmt.Errorf("add-member CONVERSATION_ID USER_ID")
		}
		method, path = "POST", "/api/conversations/"+url.PathEscape(parts[0])+"/members"
		body = map[string]string{"userId": parts[1]}
	case "remove-member", "leave":
		if (cmd == "remove-member" && len(parts) != 3) || (cmd == "leave" && len(parts) != 2) {
			return fmt.Errorf("remove-member CONVERSATION_ID USER_ID ORIGINAL_MEMBERSHIP_GENERATION | leave CONVERSATION_ID ORIGINAL_MEMBERSHIP_GENERATION")
		}
		user, generation := c.user, parts[len(parts)-1]
		if cmd == "remove-member" {
			user = parts[1]
		}
		if !decimal.MatchString(generation) {
			return fmt.Errorf("original membership generation must be an explicit positive decimal string")
		}
		if user == c.user {
			fresh, err := c.conversationRecords()
			if err != nil {
				return err
			}
			if err = c.store.prepareLeave(parts[0], generation, fresh); err != nil {
				return err
			}
		}
		method, path = "DELETE", "/api/conversations/"+url.PathEscape(parts[0])+"/members/"+url.PathEscape(user)+"?membershipGeneration="+url.QueryEscape(generation)
	case "rename":
		conversation, title, ok := strings.Cut(args, " ")
		if !ok || strings.TrimSpace(title) == "" {
			return fmt.Errorf("rename CONVERSATION_ID NEW_TITLE")
		}
		method, path = "PATCH", "/api/conversations/"+url.PathEscape(conversation)
		body = map[string]string{"title": strings.TrimSpace(title)}
	case "sync", "history":
		if len(parts) < 1 {
			return fmt.Errorf("history CONVERSATION_ID [CURSOR] | sync CONVERSATION_ID")
		}
		cursor := "0"
		if cmd == "sync" {
			var err error
			cursor, err = c.store.cursor(parts[0])
			if err != nil {
				return err
			}
		}
		if len(parts) > 1 {
			cursor = parts[1]
		}
		return c.syncHistory(parts[0], cursor)
	case "cached":
		if len(parts) != 1 {
			return fmt.Errorf("cached CONVERSATION_ID")
		}
		messages, err := c.store.history(parts[0])
		if err != nil {
			return err
		}
		return printJSON(map[string]any{"source": "local_cache", "restoreEpoch": c.epoch, "items": messages})
	case "draft":
		conversation, text, ok := strings.Cut(args, " ")
		if !ok {
			return fmt.Errorf("draft CONVERSATION_ID TEXT")
		}
		return c.store.draft(conversation, text)
	case "drafts":
		drafts, err := c.store.drafts()
		if err != nil {
			return err
		}
		return printJSON(drafts)
	case "import-outbox":
		source, deployment, ok := strings.Cut(args, " | ")
		if !ok {
			return fmt.Errorf("import-outbox PATH | CONFIRMED_DEPLOYMENT_ID; only import a source you verified belongs to this server")
		}
		return c.store.importLegacy(source, deployment)
	case "send":
		conv, text, ok := strings.Cut(args, " ")
		if !ok {
			return fmt.Errorf("send CONVERSATION_ID TEXT")
		}
		return c.send(conv, text, "", "text")
	case "retry", "reconcile":
		if len(parts) != 1 {
			return fmt.Errorf("retry CLIENT_MSG_ID")
		}
		return c.retry(parts[0], cmd == "reconcile")
	case "outbox":
		return c.showOutbox()
	case "send-media":
		if len(parts) < 3 {
			return fmt.Errorf("send-media CONVERSATION_ID image|audio|video|file OBJECT_ID [CAPTION]")
		}
		if !map[string]bool{"image": true, "audio": true, "video": true, "file": true}[parts[1]] {
			return fmt.Errorf("unsupported media kind")
		}
		return c.queueAndSend(pending{ConversationID: parts[0], Type: parts[1], AttachmentID: parts[2], Text: strings.Join(parts[3:], " ")})
	case "reply":
		conversation, rest, ok := strings.Cut(args, " ")
		source, text, ok2 := strings.Cut(rest, " ")
		if !ok || !ok2 {
			return fmt.Errorf("reply CONVERSATION_ID SOURCE_MESSAGE_ID TEXT")
		}
		return c.queueAndSend(pending{ConversationID: conversation, Type: "text", ReplyToID: source, Text: text})
	case "forward":
		if len(parts) != 3 {
			return fmt.Errorf("forward CONVERSATION_ID SOURCE_MESSAGE_ID SOURCE_CONTENT_VERSION; inspect with message SOURCE_MESSAGE_ID first")
		}
		return c.forwardMessage(parts[0], parts[1], parts[2])
	case "message":
		if len(parts) != 1 {
			return fmt.Errorf("message MESSAGE_ID")
		}
		path = "/api/messages/" + url.PathEscape(parts[0])
	case "download-forward":
		object, destination, ok := strings.Cut(args, " | ")
		if !ok {
			return fmt.Errorf("download-forward MESSAGE_ID | OUTPUT_PATH")
		}
		return c.downloadForward(object, destination)
	case "recovery-codes":
		return c.recoveryCodes()
	case "download":
		object, destination, ok := strings.Cut(args, " | ")
		if !ok {
			return fmt.Errorf("download OBJECT_ID | OUTPUT_PATH (existing files are retained)")
		}
		return c.download(object, destination)
	case "sessions":
		path = "/api/auth/sessions"
	case "revoke":
		if len(parts) != 1 {
			return fmt.Errorf("revoke DEVICE_ID")
		}
		method = "DELETE"
		path = "/api/auth/sessions/" + url.PathEscape(parts[0])
	case "block", "unblock":
		if len(parts) != 1 {
			return fmt.Errorf("block|unblock USER_ID")
		}
		method = "PUT"
		if cmd == "unblock" {
			method = "DELETE"
		} else {
			fresh, err := c.conversationRecords()
			if err != nil {
				return err
			}
			if err = c.store.prepareRelationshipRevocation(parts[0], "PEER_BLOCKED", fresh); err != nil {
				return err
			}
		}
		path = "/api/blocks/" + url.PathEscape(parts[0])
	case "unpin", "unfavorite":
		if len(parts) != 1 {
			return fmt.Errorf("unpin|unfavorite MESSAGE_ID")
		}
		method = "DELETE"
		path = "/api/messages/" + url.PathEscape(parts[0]) + "/" + strings.TrimPrefix(cmd, "un")
	case "upload":
		return c.upload(args)
	case "edit":
		mid, rest, ok := strings.Cut(args, " ")
		ver, text, ok2 := strings.Cut(rest, " ")
		if !ok || !ok2 {
			return fmt.Errorf("edit MESSAGE_ID EXPECTED_VERSION TEXT")
		}
		method = "PATCH"
		path = "/api/messages/" + url.PathEscape(mid)
		body = map[string]string{"text": text, "expectedVersion": ver, "requestId": id()}
	case "retract":
		method = "DELETE"
		path = "/api/messages/" + url.PathEscape(args)
	case "react":
		if len(parts) != 2 {
			return fmt.Errorf("react MESSAGE_ID EMOJI")
		}
		method = "POST"
		path = "/api/messages/" + url.PathEscape(parts[0]) + "/reactions"
		body = map[string]string{"emoji": parts[1]}
	case "pin", "favorite":
		method = "PUT"
		path = "/api/messages/" + args + "/" + cmd
	case "logout":
		if c.auth != nil {
			return c.auth.logout()
		}
		method = "POST"
		path = "/api/auth/logout"
	default:
		return fmt.Errorf("unknown command")
	}
	if e := c.call(method, path, body, &result); e != nil {
		return e
	}
	if cmd == "logout" {
		c.token = ""
	}
	return printJSON(result)
}

func (c *client) upload(path string) error {
	f, e := os.Open(path)
	if e != nil {
		return e
	}
	defer f.Close()
	var b bytes.Buffer
	writer := multipart.NewWriter(&b)
	part, e := writer.CreateFormFile("file", filepath.Base(path))
	if e != nil {
		return e
	}
	if _, e = io.Copy(part, io.LimitReader(f, (64<<20)+1)); e != nil {
		return e
	}
	if b.Len() > 64<<20 {
		return fmt.Errorf("upload exceeds 64 MiB")
	}
	writer.Close()
	res, e := c.authenticatedResponse("POST", "/api/media", writer.FormDataContentType(), b.Bytes(), 20*time.Second)
	if e != nil {
		return e
	}
	defer res.Body.Close()
	data, e := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if e != nil {
		return e
	}
	if res.StatusCode >= 300 {
		return fmt.Errorf("upload HTTP %d: %s", res.StatusCode, data)
	}
	fmt.Println(string(data))
	return nil
}
