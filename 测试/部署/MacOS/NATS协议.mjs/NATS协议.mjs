import net from 'node:net';
import fs from 'node:fs';
import assert from 'node:assert/strict';
import crypto from 'node:crypto';
import { execFileSync } from 'node:child_process';

const port = Number(process.env.JOBS_IM_NATS_PORT);
const token = fs.readFileSync(process.env.JOBS_IM_NATS_TOKEN_FILE, 'utf8').trim();
const mode = process.argv[2];
const suffix = process.env.JOBS_IM_NATS_TEST_ID;
assert.match(suffix, /^[a-z0-9_]+$/);
const stream = 'DEPLOY_' + suffix, subject = 'deployment.' + suffix;
class Wire {
  constructor(secret) { this.secret = secret; this.bytes = Buffer.alloc(0); this.replies = new Map(); this.sid = 0; }
  async connect() {
    this.socket = net.connect({ host: '127.0.0.1', port });
    this.socket.setTimeout(2500, () => this.socket.destroy(Error('test timeout')));
    this.socket.on('data', bytes => { this.bytes = Buffer.concat([this.bytes, bytes]); this.parse(); });
    this.socket.on('error', () => { this.failed = true; });
    await new Promise((resolve, reject) => { this.socket.once('connect', resolve); this.socket.once('error', reject); });
    this.socket.write('CONNECT ' + JSON.stringify({ auth_token: this.secret, verbose: true }) + '\r\nPING\r\n');
    for (let round = 0; round < 100 && !this.pong && !this.failed; round++) await new Promise(resolve => setTimeout(resolve, 10));
    return this.pong === true && !this.failed;
  }
  parse() {
    for (;;) {
      const end = this.bytes.indexOf('\r\n'); if (end < 0) return;
      const line = this.bytes.subarray(0, end).toString('ascii');
      if (line.startsWith('MSG ')) {
        const parts = line.split(' '), size = Number(parts.at(-1));
        if (this.bytes.length < end + 2 + size + 2) return;
        const payload = this.bytes.subarray(end + 2, end + 2 + size);
        this.bytes = this.bytes.subarray(end + 2 + size + 2);
        const waiter = this.replies.get(parts[2]);
        if (waiter) { this.replies.delete(parts[2]); waiter.resolve(JSON.parse(payload.toString())); }
        continue;
      }
      this.bytes = this.bytes.subarray(end + 2);
      if (line.startsWith('INFO ')) { this.info = JSON.parse(line.slice(5)); }
      if (line === 'PING') this.socket.write('PONG\r\n');
      if (line === 'PONG') this.pong = true;
      if (line.startsWith('-ERR')) this.failed = true;
    }
  }
  request(topic, value) {
    const sid = String(++this.sid), inbox = '_INBOX.' + crypto.randomUUID().replaceAll('-', '');
    const body = Buffer.from(typeof value === 'string' ? value : JSON.stringify(value));
    return new Promise((resolve, reject) => {
      const timeout = setTimeout(() => { this.replies.delete(sid); reject(Error('JetStream request timeout')); }, 2500);
      this.replies.set(sid, { resolve: result => { clearTimeout(timeout); resolve(result); } });
      this.socket.write('SUB ' + inbox + ' ' + sid + '\r\nUNSUB ' + sid + ' 1\r\nPUB ' + topic + ' ' + inbox + ' ' + body.length + '\r\n');
      this.socket.write(body); this.socket.write('\r\n');
    });
  }
  close() { this.socket?.destroy(); }
}
for (const secret of ['', 'wrong-test-token']) {
  const denied = new Wire(secret);
  try { assert.equal(await denied.connect(), false); } finally { denied.close(); }
}
const client = new Wire(token);
try {
  assert.equal(await client.connect(), true); assert.equal(client.info.jetstream, true);
  if (mode === 'create') {
    const command = execFileSync('/bin/ps', ['-ww', '-p', process.env.JOBS_IM_NATS_TEST_PID, '-o', 'command='], { encoding: 'utf8' });
    assert.equal(command.includes(token), false);
    assert.equal(fs.readFileSync(process.env.JOBS_IM_NATS_TEST_LOG, 'utf8').includes(token), false);
    const created = await client.request('$JS.API.STREAM.CREATE.' + stream, { name: stream, subjects: [subject], storage: 'file', retention: 'limits', max_bytes: 1048576, num_replicas: 1 });
    assert.equal(created.error, undefined); assert.equal(created.config.storage, 'file');
    const ack = await client.request(subject, 'persistent-test-payload');
    assert.equal(ack.stream, stream); assert.equal(ack.seq, 1);
  } else if (mode === 'verify') {
    const message = await client.request('$JS.API.STREAM.MSG.GET.' + stream, { seq: 1 });
    assert.equal(message.error, undefined);
    assert.equal(Buffer.from(message.message.data, 'base64').toString(), 'persistent-test-payload');
  } else throw Error('Unknown test mode');
  console.log('PASS ' + mode + ': missing/wrong-token rejected, authenticated JetStream ' + (mode === 'create' ? 'file publication' : 'restart persistence'));
} finally { client.close(); }
