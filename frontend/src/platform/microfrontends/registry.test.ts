import {describe,expect,it} from 'vitest';
import {moduleRegistry,visibleModules} from './registry';
describe('microfrontend registry',()=>{
  it('expone Onsite para la capability requerida',()=>{expect(visibleModules(['onsite:read']).map(item=>item.id)).toContain('onsite-operations')});
  it('oculta módulos cuando no existe capability',()=>{const registry=moduleRegistry.map(item=>item.id==='onsite-operations'?{...item,capabilities:['platform:admin' as const]}:item);expect(visibleModules(['onsite:read'],registry)).not.toContainEqual(expect.objectContaining({id:'onsite-operations'}))});
});
