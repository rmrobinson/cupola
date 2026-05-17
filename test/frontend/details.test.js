const test = require('node:test');
const assert = require('node:assert/strict');
const fs = require('node:fs');
const path = require('node:path');
const vm = require('node:vm');

class FakeElement {
  constructor(tag) {
    this.tag = tag;
    this.children = [];
    this.className = '';
    this.id = '';
    this.attributes = {};
    this.innerHTML = '';
    this.classList = {
      add: cls => {
        const classes = new Set(this.className.split(/\s+/).filter(Boolean));
        classes.add(cls);
        this.className = [...classes].join(' ');
      },
      remove: cls => {
        const classes = new Set(this.className.split(/\s+/).filter(Boolean));
        classes.delete(cls);
        this.className = [...classes].join(' ');
      },
      contains: cls => this.className.split(/\s+/).includes(cls),
    };
  }

  appendChild(child) {
    this.children.push(child);
    return child;
  }

  addEventListener() {}

  setAttribute(name, value) {
    this.attributes[name] = value;
  }

  querySelector(selector) {
    if (selector === '.detail-close') return { focus() {} };
    return null;
  }
}

function loadDetails() {
  const body = new FakeElement('body');
  const context = {
    window: { location: { href: 'http://localhost/' } },
    document: {
      body,
      activeElement: { focus() {} },
      createElement: tag => new FakeElement(tag),
      addEventListener() {},
    },
    Date,
    URL,
    console,
  };
  vm.createContext(context);
  const file = path.join(__dirname, '../../cmd/cupola/frontend/js/details.js');
  vm.runInContext(fs.readFileSync(file, 'utf8'), context);
  return { context, body };
}

test('detail drawer renders numeric zero with percent unit', () => {
  const { context, body } = loadDetails();

  context.window.CupolaDetails.show({
    domain: 'weather.forecast.hourly',
    title: '1:00 PM',
    fields: [
      { key: 'precip_chance', value: 0, unit: 'percent' },
    ],
  });

  assert.match(body.children[0].innerHTML, /0%/);
  assert.doesNotMatch(body.children[0].innerHTML, />%<\/dd>/);
});
