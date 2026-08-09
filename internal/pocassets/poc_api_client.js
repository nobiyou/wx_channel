(function () {
  'use strict';
  const config = window.__WX_CHANNEL_POC_CONFIG__;
  const allowed = new Set(['finderSearch', 'finderGetCommentDetail', 'finderGetCommentList']);
  const ws = new WebSocket(`ws://127.0.0.1:${config.port}/ws/api`, ['wx-poc-v1', `auth.${config.token}`]);

  function methods() {
    return {
      finderSearch: !!(window.WXU && WXU.API2 && typeof WXU.API2.finderSearch === 'function'),
      finderGetCommentDetail: !!(window.WXU && WXU.API && typeof WXU.API.finderGetCommentDetail === 'function'),
      finderGetCommentList: !!(window.WXU && WXU.API && typeof WXU.API.finderGetCommentList === 'function')
    };
  }

  function send(type, data) {
    if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify({type, data}));
  }

  function state() {
    send('client_state', {pagePath: location.pathname, visible: !document.hidden, methods: methods(), timestamp: Date.now()});
  }

  async function invoke(method, body) {
    if (!allowed.has(method)) throw new Error('method_not_allowed');
    if (method === 'finderSearch') return WXU.API2.finderSearch({query: body.keyword, scene: 19, requestId: String(Date.now()), lastBuffer: body.next_marker || '', lastBuff: body.next_marker || ''});
    if (method === 'finderGetCommentDetail') return WXU.API.finderGetCommentDetail({needObject: 1, lastBuffer: '', scene: 146, direction: 2, identityScene: 2, pullScene: 6, objectid: String(body.object_id).split('_')[0], objectNonceId: body.nonce_id, encrypted_objectid: ''});
    const payload = body.comment_id ? {direction: 2, identityScene: 2, objectId: body.object_id, rootCommentId: body.comment_id, lastBuffer: body.next_marker || undefined} : {finderBasereq: {scene: 140, ctxInfo: {clientReportBuff: '{"entranceId":"1002"}'}, objectBaseInfos: []}, objectId: body.object_id, objectNonceId: body.nonce_id, direction: 2, identityScene: 2, lastBuffer: body.next_marker || undefined, enterSessionId: String(Date.now())};
    return WXU.API.finderGetCommentList(payload);
  }

  ws.onopen = state;
  ws.onmessage = async (event) => {
    const message = JSON.parse(event.data);
    if (message.type === 'ping') {
      send('pong', {});
      return;
    }
    if (message.type !== 'api_call') return;
    try {
      send('api_response', {id: message.data.id, data: await invoke(message.data.method, message.data.body), errCode: 0});
    } catch (error) {
      const text = String(error && error.message || '');
      const targetMismatch = text.includes('-70003') || text.includes('JSAPI_JSONPARSE_FAILED');
      send('api_response', {id: message.data.id, errCode: targetMismatch ? -70003 : 1011, errMsg: targetMismatch ? 'target_context_mismatch' : 'page_api_failed'});
    }
  };
  document.addEventListener('visibilitychange', state);
}());
