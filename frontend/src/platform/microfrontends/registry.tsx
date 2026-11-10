import {BarChart3,CalendarCheck2,ClipboardCheck,LayoutDashboard,ListChecks} from 'lucide-react';
import type {MicrofrontendManifest} from './contracts';
import {canMount} from './contracts';
import {OnsiteModule} from '../../microfrontends/onsite-operations/OnsiteModule';
export const moduleRegistry:MicrofrontendManifest[]=[
  {id:'overview',name:'Inicio',description:'Contexto de tu organización',group:'Plataforma',defaultRoute:'/',routes:['/'],capabilities:[],icon:<LayoutDashboard size={17}/>,load:async()=>()=>null},
  {id:'onsite-operations',name:'Onsite Operations',description:'Check-in y operación presencial',group:'Operaciones',defaultRoute:'/onsite',routes:['/onsite','/onsite/attendance','/onsite/reports'],capabilities:['onsite:read'],icon:<CalendarCheck2 size={17}/>,children:[{id:'onsite-checkin',label:'Control de acceso',description:'Registro de entradas',route:'/onsite',icon:<ClipboardCheck size={16}/>},{id:'onsite-attendance',label:'Asistencia',description:'Seguimiento de participantes',route:'/onsite/attendance',icon:<ListChecks size={16}/>},{id:'onsite-reports',label:'Reportes',description:'Resultados y HukID',route:'/onsite/reports',icon:<BarChart3 size={16}/>}],load:async()=>OnsiteModule},
];
export function visibleModules(capabilities:MicrofrontendManifest['capabilities'][number][],registry=moduleRegistry){return registry.filter(module=>canMount(module,capabilities));}
