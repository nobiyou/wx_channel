'use strict';

const assert = require('assert');
const fs = require('fs');
const path = require('path');

const source = fs.readFileSync(path.join(__dirname, 'poc_api_client.js'), 'utf8');

for (const method of ['finderSearch', 'finderUserPage', 'finderGetCommentDetail', 'finderGetCommentList']) {
  assert(source.includes(method), `missing allowed method ${method}`);
}
for (const forbidden of ['console.', 'fetch(', 'commentLike', 'commentPost', 'finderPost', 'publish', 'download', '?token=']) {
  assert(!source.includes(forbidden), `forbidden bridge capability ${forbidden}`);
}
assert(source.includes("'target_context_mismatch'"));
assert(source.includes("'page_api_failed'"));
assert(!/errMsg\s*:\s*(?:text|error\.message)/.test(source), 'raw error text is sent to the bridge');
