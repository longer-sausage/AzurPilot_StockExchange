import {useEffect,useRef,useState,type ReactNode} from 'react'
import {createPortal} from 'react-dom'
import {X} from 'lucide-react'
export function Modal({title,children,onClose}:{title:string;children:ReactNode;onClose?:()=>void}){
 const panel=useRef<HTMLDivElement>(null),close=useRef(onClose);close.current=onClose
 const [host]=useState(()=>{const el=document.createElement('div');el.dataset.mmexModalHost='true';return el}),[viewport,setViewport]=useState<{top:number;left:number;width:number;height:number}>()
 useEffect(()=>{const old=document.activeElement as HTMLElement|null,overflow=document.body.style.overflow;document.body.append(host);const siblings=Array.from(document.body.children).filter((el):el is HTMLElement=>el instanceof HTMLElement&&el!==host).map(el=>({el,inert:el.inert}));siblings.forEach(x=>x.el.inert=true);document.body.style.overflow='hidden'
  const controls=()=>Array.from(panel.current?.querySelectorAll<HTMLElement>('button:not(:disabled),input:not(:disabled),a[href],select:not(:disabled),textarea:not(:disabled),[tabindex="0"]')??[]).filter(el=>el.getClientRects().length>0)
  const top=()=>Array.from(document.querySelectorAll('[data-mmex-modal-host]')).at(-1)===host
  const first=()=>{(controls()[0]??panel.current)?.focus()};first()
  function key(e:KeyboardEvent){if(!top())return;if(e.key==='Escape'){e.stopPropagation();close.current?.()}if(e.key==='Tab'){const items=controls();if(!items.length){e.preventDefault();return};const index=items.indexOf(document.activeElement as HTMLElement);if(e.shiftKey&&index<=0){e.preventDefault();items.at(-1)?.focus()}else if(!e.shiftKey&&(index===items.length-1||index<0)){e.preventDefault();items[0].focus()}}}
  const focus=(e:FocusEvent)=>{if(top()&&!panel.current?.contains(e.target as Node))first()}
  const resize=()=>{const v=window.visualViewport;setViewport(v?{top:v.offsetTop,left:v.offsetLeft,width:v.width,height:v.height}:undefined)};resize();window.visualViewport?.addEventListener('resize',resize);window.visualViewport?.addEventListener('scroll',resize);document.addEventListener('keydown',key);document.addEventListener('focusin',focus)
  return()=>{document.removeEventListener('keydown',key);document.removeEventListener('focusin',focus);window.visualViewport?.removeEventListener('resize',resize);window.visualViewport?.removeEventListener('scroll',resize);siblings.forEach(x=>x.el.inert=x.inert);document.body.style.overflow=overflow;host.remove();old?.focus()}
 },[host])
 return createPortal(<div className="modal-backdrop" style={viewport?{top:viewport.top,left:viewport.left,width:viewport.width,height:viewport.height,bottom:'auto',right:'auto'}:undefined} onMouseDown={e=>{if(e.target===e.currentTarget)close.current?.()}}><div ref={panel} className="modal" role="dialog" aria-modal="true" aria-label={title} tabIndex={-1}><div className="modal-title"><h2>{title}</h2>{onClose&&<button className="icon" onClick={onClose} aria-label="关闭"><X size={20}/></button>}</div><div className="modal-body">{children}</div></div></div>,host)
}
export function Alert({children}:{children:ReactNode}){return <div role="alert" className="alert">{children}</div>}
export function Stat({label,value,suffix,note}:{label:string;value:string;suffix?:string;note?:string}){return <section className="stat"><span>{label}</span><strong>{value}<small>{suffix}</small></strong>{note&&<small>{note}</small>}</section>}
export function Empty({title,text}:{title:string;text:string}){return <div className="empty"><p>{title}</p><small>{text}</small></div>}
