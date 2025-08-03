import '../less/knot.css';

import Alpine from 'alpinejs';
import persist from '@alpinejs/persist';
import AlpineFloatingUI from "@awcodes/alpine-floating-ui";
import focus from '@alpinejs/focus';

import {} from './timezones.js';
import {} from './components/autocompleter.js';

import md5 from 'crypto-js/md5';

import './pages/initialUserForm.js';
import './pages/loginUserForm.js';
import './pages/userGroupForm.js';
import './pages/createTokenForm.js';
import './pages/apiTokensComponent.js';
import './pages/groupListComponent.js';
import './pages/rolesListComponent.js';
import './pages/userRolesForm.js';
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
import './pages/usageComponent.js';
import './pages/tunnelsListComponent.js';
import './pages/auditLogComponent.js';
import './pages/clusterInfoComponent.js';
import './components/chat.js';

window.Alpine = Alpine;
Alpine.plugin(persist);
Alpine.plugin(AlpineFloatingUI);
Alpine.plugin(focus);
Alpine.start();

window.MD5 = function(str) {
  return md5(str).toString();
}
