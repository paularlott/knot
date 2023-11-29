import Alpine from 'alpinejs'
import { validate } from './validators.js'

window.validate = validate;

Alpine.start()

window.openTerminal = function(space, spaceId) {
// TODO  window.open('/terminal.html', 'spaces_' + spaceId + '_terminal', 'width=800,height=600');
  window.open('/terminal/' + space, 'spaces_' + spaceId + '_terminal', 'width=800,height=500');
  return false;
}
