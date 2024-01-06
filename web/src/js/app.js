import Alpine from 'alpinejs'
import { validate } from './validators.js'

window.validate = validate;

Alpine.start()

window.openTerminal = function(spaceId) {
  window.open('/terminal/' + spaceId, 'spaces_' + spaceId + '_terminal', 'width=800,height=500');
  return false;
}

/// wysiwyg editor
import 'ace-builds/src-noconflict/ace';
import 'ace-builds/src-noconflict/mode-terraform';
import 'ace-builds/src-noconflict/mode-yaml';
import 'ace-builds/src-noconflict/theme-github';
import 'ace-builds/src-noconflict/theme-github_dark';
