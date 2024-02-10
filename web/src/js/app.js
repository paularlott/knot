import '../less/app.less';

import Alpine from 'alpinejs';
import persist from '@alpinejs/persist';
import { validate } from './validators.js';
import {} from './timezones.js';
import {} from './components/autocompleter.js';

import './pages/createInitialUserForm.js';
import './pages/loginUserForm.js';
import './pages/userGroupForm.js';
import './pages/createTokenForm.js';
import './pages/apiTokensComponent.js';
import './pages/groupListComponent.js';
import './pages/sessionsListComponent.js';
import './pages/templateListComponent.js';
import './pages/templateForm.js';
import './pages/userListComponent.js';
import './pages/userForm.js';
import './pages/templateVarListComponent.js';
import './pages/variableForm.js';
import './pages/volumeListComponent.js';
import './pages/volumeForm.js';
import './pages/spaceForm.js';
import './pages/spacesListComponent.js';

window.validate = validate;
window.focusElement = function(selector) {
  setTimeout(() => {
    document.querySelector(selector).focus();
  }, 10);
}

window.Alpine = Alpine;
Alpine.plugin(persist)
Alpine.start();

window.openTerminal = function(spaceId) {
  window.open('/terminal/' + spaceId, 'spaces_' + spaceId + '_terminal', 'width=800,height=500');
  return false;
}

window.openCodeServer = function(spaceId) {
  const maxWidth = Math.min(window.innerWidth, 1440);
  const maxHeight = window.innerHeight;
  window.open('/proxy/spaces/' + spaceId + '/code-server/', 'spaces_' + spaceId + '_code_server', 'width=' + maxWidth + ',height=' + maxHeight);
  return false;
}

window.openVNC = function(spaceId, domain, username, spaceName) {
  const subdomain = window.location.protocol + '//' + username + '--' + spaceName + '--vnc';
  const maxWidth = Math.min(window.innerWidth, 1440);
  const maxHeight = window.innerHeight;

  window.open(domain.replace(/^\*/, subdomain), 'spaces_' + spaceId + '_vnc', 'width=' + maxWidth + ',height=' + maxHeight);
  return false;
}

window.openPortWindow = function(spaceId, domain, username, spaceName, port) {
  var subdomain = window.location.protocol + '//' + username + '--' + spaceName + '--' + port;
  window.open(domain.replace(/^\*/, subdomain), 'spaces_' + spaceId + '_http_port_' + port);
  return false;
}

/// wysiwyg editor
import 'ace-builds/src-noconflict/ace';
import 'ace-builds/src-noconflict/mode-terraform';
import 'ace-builds/src-noconflict/mode-yaml';
import 'ace-builds/src-noconflict/mode-text';
import 'ace-builds/src-noconflict/theme-github';
import 'ace-builds/src-noconflict/theme-github_dark';
import 'ace-builds/src-noconflict/ext-searchbox';
