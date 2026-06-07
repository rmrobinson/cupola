const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

function loadStream() {
  const instances = [];
  const ids = ['seed-session', 'stream-1', 'stream-2', 'stream-3'];

  class FakeEventSource {
    static CONNECTING = 0;
    static OPEN = 1;
    static CLOSED = 2;

    constructor(url) {
      this.url = url;
      this.closed = false;
      this.readyState = FakeEventSource.OPEN;
      instances.push(this);
    }

    close() {
      this.closed = true;
      this.readyState = FakeEventSource.CLOSED;
    }
  }

  const context = {
    crypto: { randomUUID: () => ids.shift() },
    EventSource: FakeEventSource,
    setTimeout: () => 1,
    clearTimeout: () => {},
    setInterval: () => 1,
    clearInterval: () => {},
    document: { getElementById: () => null },
  };
  vm.createContext(context);

  const file = path.join(__dirname, '../../cmd/cupola/frontend/js/stream.js');
  vm.runInContext(fs.readFileSync(file, 'utf8') + '\nthis.__Stream = Stream;', context);

  return { Stream: context.__Stream, instances };
}

test('reconnectNow opens a fresh SSE session and closes the previous stream', () => {
  const { Stream, instances } = loadStream();

  Stream.connect();
  assert.equal(Stream.SESSION_ID, 'stream-1');
  assert.equal(instances[0].url, '/api/v1/stream?session_id=stream-1');
  assert.equal(instances[0].closed, false);

  Stream.reconnectNow();
  assert.equal(instances[0].closed, true);
  assert.equal(Stream.SESSION_ID, 'stream-2');
  assert.equal(instances[1].url, '/api/v1/stream?session_id=stream-2');
});

test('stale stream open callbacks are ignored after reconnect', () => {
  const { Stream, instances } = loadStream();
  let opens = 0;
  Stream.onConnect(() => { opens++; });

  Stream.connect();
  const stale = instances[0];
  Stream.reconnectNow();

  stale.onopen();
  assert.equal(opens, 0);

  instances[1].onopen();
  assert.equal(opens, 1);
});

test('resume reconnect is skipped while the active stream is open', () => {
  const { Stream, instances } = loadStream();

  assert.equal(Stream.shouldReconnectOnResume(), true);

  Stream.connect();
  assert.equal(Stream.shouldReconnectOnResume(), false);

  instances[0].readyState = instances[0].constructor.CLOSED;
  assert.equal(Stream.shouldReconnectOnResume(), true);
});
