const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

function loadStream() {
  const instances = [];
  const ids = ['seed-session', 'stream-1', 'stream-2', 'stream-3'];

  class FakeEventSource {
    constructor(url) {
      this.url = url;
      this.closed = false;
      instances.push(this);
    }

    close() {
      this.closed = true;
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
