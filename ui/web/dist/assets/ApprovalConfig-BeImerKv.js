import{u as J,r as c,j as e,B as n,C as M,T as r,de as W,a as g,A as N,D as A,b as y,P as V,c as G,V as $,d as D,I as q,g as K,h as Q,i as X,k as Y,l as Z,m as o,F as ee,n as ae,o as le,M as k,b9 as v,_ as te,p as se,df as h}from"./index-D6BTcUK7.js";import{S as f}from"./Stack-XE36b3Ws.js";import{T as ie,a as re,b as ne,c as H,d as i,e as oe}from"./TableRow-CYpFYjHS.js";import{C as w}from"./Chip-CivcMOwo.js";import{A as _,a as x,b as m}from"./AccordionSummary-CpRD2eRJ.js";import{S as pe}from"./Switch-h5Y-bKMA.js";const p={feishu:`[
  {
    "id": "widget17424405077590001",
    "name": "工单标题",
    "type": "input",
    "required": false,
    "visible": true,
    "printable": true,
    "enable_default_value": false,
    "default_value_type": "",
    "widget_default_value": "",
    "display_condition": null
  },
  {
    "id": "widget17611283640360001",
    "name": "详细描述",
    "type": "textarea",
    "required": false,
    "visible": true,
    "printable": true,
    "enable_default_value": false,
    "default_value_type": "",
    "widget_default_value": "",
    "display_condition": null
  },
  {
    "id": "widget17611425102270001",
    "name": "申请类型",
    "type": "radioV2",
    "required": false,
    "visible": true,
    "printable": true,
    "enable_default_value": false,
    "default_value_type": "",
    "widget_default_value": "",
    "display_condition": null,
    "option": [
      {"value": "mh22s1w3-nkqsak2eis-0", "text": "host_access"},
      {"value": "mh22s1w3-dipw4j04vb-0", "text": "host_group_access"}
    ]
  },
  {
    "id": "widget17611283900470001",
    "name": "申请理由",
    "type": "textarea",
    "required": false,
    "visible": true,
    "printable": true,
    "enable_default_value": false,
    "default_value_type": "",
    "widget_default_value": "",
    "display_condition": null
  },
  {
    "id": "widget17611284241860001",
    "name": "申请资源",
    "type": "textarea",
    "required": false,
    "visible": true,
    "printable": true,
    "enable_default_value": false,
    "default_value_type": "",
    "widget_default_value": "",
    "display_condition": null
  },
  {
    "id": "widget17611477809060001",
    "name": "权限时长",
    "type": "input",
    "required": false,
    "visible": true,
    "printable": true,
    "enable_default_value": false,
    "default_value_type": "",
    "widget_default_value": "",
    "display_condition": null
  }
]`,dingtalk:`[
  {
    "name": "title",
    "label": "工单标题",
    "field": "title"
  },
  {
    "name": "description",
    "label": "详细描述",
    "field": "description"
  },
  {
    "name": "type",
    "label": "申请类型",
    "field": "type",
    "options": [
      {"value": "host_access", "label": "主机访问权限"},
      {"value": "host_group_access", "label": "主机组访问权限"}
    ]
  },
  {
    "name": "reason",
    "label": "申请理由",
    "field": "reason"
  },
  {
    "name": "duration",
    "label": "权限时长",
    "field": "duration"
  }
]`,wechat:`[
  {
    "control": "Text",
    "id": "Text-title",
    "label": "工单标题",
    "field": "title"
  },
  {
    "control": "Textarea",
    "id": "Textarea-description",
    "label": "详细描述",
    "field": "description"
  },
  {
    "control": "Select",
    "id": "Select-type",
    "label": "申请类型",
    "field": "type",
    "options": [
      {"value": "host_access", "label": "主机访问权限"},
      {"value": "host_group_access", "label": "主机组访问权限"}
    ]
  },
  {
    "control": "Textarea",
    "id": "Textarea-reason",
    "label": "申请理由",
    "field": "reason"
  },
  {
    "control": "Text",
    "id": "Text-duration",
    "label": "权限时长",
    "field": "duration"
  }
]`};function he(){const{t:l}=J(),[U,T]=c.useState(!1),[I,z]=c.useState([]),[E,S]=c.useState(!1),[d,b]=c.useState(null),[t,s]=c.useState({name:"",type:"feishu",enabled:!0,app_id:"",app_secret:"",approval_code:"",process_code:"",template_id:"",form_fields:p.feishu,approver_user_ids:"",cc_user_ids:"",cc_open_ids:"",api_base_url:"",callback_url:""}),C=async()=>{T(!0);try{const a=await h.getApprovalConfig();console.log("加载到的工单配置列表：",a),z(a.configs||[])}catch(a){console.error("Failed to load approval configs:",a)}finally{T(!1)}};c.useEffect(()=>{C()},[]);const F=a=>{a?(console.log("编辑配置，原始数据：",a),b(a),s({name:a.name||"",type:a.type||"feishu",enabled:a.enabled!==void 0?a.enabled:!0,app_id:a.app_id||"",app_secret:a.app_secret||"",approval_code:a.approval_code||"",process_code:a.process_code||"",template_id:a.template_id||"",form_fields:a.form_fields||p[a.type||"feishu"],approver_user_ids:a.approver_user_ids||"",cc_user_ids:a.cc_user_ids||"",cc_open_ids:a.cc_open_ids||"",api_base_url:a.api_base_url||"",callback_url:a.callback_url||""})):(b(null),s({name:"",type:"feishu",enabled:!0,app_id:"",app_secret:"",approval_code:"",process_code:"",template_id:"",form_fields:p.feishu,approver_user_ids:"",cc_user_ids:"",cc_open_ids:"",api_base_url:"",callback_url:""})),S(!0)},j=()=>{S(!1),b(null)},B=async()=>{try{try{JSON.parse(t.form_fields||"[]")}catch{alert(l("settings.approvalConfig.invalidJsonFormat"));return}const a={name:t.name,type:t.type,enabled:t.enabled,app_id:t.app_id,app_secret:t.app_secret,approval_code:t.approval_code,process_code:t.process_code,template_id:t.template_id,form_fields:t.form_fields,approver_user_ids:t.approver_user_ids,cc_user_ids:t.cc_user_ids,cc_open_ids:t.cc_open_ids,api_base_url:t.api_base_url,callback_url:t.callback_url};d?(await h.updateApprovalConfig(d.id,a),alert(l("settings.approvalConfig.messages.updateSuccess"))):(await h.updateApprovalConfig(null,a),alert(l("settings.approvalConfig.messages.createSuccess"))),j(),C()}catch(a){console.error("Failed to save config:",a),alert(l("settings.approvalConfig.messages.saveFailed")+": "+a.message)}},P=async a=>{if(confirm(l("settings.approvalConfig.deleteConfirm")))try{await h.deleteApprovalConfig(a),alert(l("settings.approvalConfig.messages.deleteSuccess")),C()}catch(u){console.error("Failed to delete config:",u),alert(l("settings.approvalConfig.messages.saveFailed")+": "+u.message)}},L=a=>{s({...t,type:a,form_fields:p[a],approval_code:"",process_code:"",template_id:""})},O=a=>({feishu:l("settings.approvalConfig.platforms.feishu"),dingtalk:l("settings.approvalConfig.platforms.dingtalk"),wechat:l("settings.approvalConfig.platforms.wechat")})[a]||a,R=a=>a?new Date(a).toLocaleString():"-";return U?e.jsx(n,{sx:{display:"flex",justifyContent:"center",py:4},children:e.jsx(M,{})}):e.jsx(n,{sx:{p:3},children:e.jsxs(f,{spacing:4,children:[e.jsxs(n,{children:[e.jsxs(n,{sx:{display:"flex",justifyContent:"space-between",alignItems:"center",mb:3},children:[e.jsxs(n,{children:[e.jsxs(r,{variant:"h5",fontWeight:600,sx:{display:"flex",alignItems:"center"},children:[e.jsx(W,{sx:{mr:1}})," ",l("settings.approvalConfig.title")]}),e.jsx(r,{variant:"body2",color:"text.secondary",sx:{mt:.5},children:l("settings.approvalConfig.subtitle")})]}),e.jsx(g,{variant:"contained",startIcon:e.jsx(N,{}),onClick:()=>F(),children:l("settings.approvalConfig.addConfig")})]}),e.jsx(A,{})]}),e.jsxs(y,{severity:"info",icon:e.jsx(W,{}),children:[e.jsx(r,{variant:"body2",fontWeight:"bold",gutterBottom:!0,children:l("settings.approvalConfig.tips.title")}),e.jsxs(r,{variant:"body2",component:"div",children:["• ",e.jsx("strong",{children:l("settings.approvalConfig.platforms.feishu")}),": ",l("settings.approvalConfig.tips.feishu"),e.jsx("br",{}),"• ",e.jsx("strong",{children:l("settings.approvalConfig.platforms.dingtalk")}),": ",l("settings.approvalConfig.tips.dingtalk"),e.jsx("br",{}),"• ",e.jsx("strong",{children:l("settings.approvalConfig.platforms.wechat")}),": ",l("settings.approvalConfig.tips.wechat")]})]}),I.length===0?e.jsx(y,{severity:"info",children:l("settings.approvalConfig.noConfigs")}):e.jsx(ie,{component:V,variant:"outlined",children:e.jsxs(re,{children:[e.jsx(ne,{children:e.jsxs(H,{children:[e.jsx(i,{children:l("settings.approvalConfig.table.name")}),e.jsx(i,{children:l("settings.approvalConfig.table.platform")}),e.jsx(i,{children:l("settings.approvalConfig.table.appId")}),e.jsx(i,{children:l("settings.approvalConfig.table.flowCode")}),e.jsx(i,{align:"center",children:l("settings.approvalConfig.table.status")}),e.jsx(i,{align:"center",children:l("settings.approvalConfig.table.createdAt")}),e.jsx(i,{align:"right",children:l("settings.approvalConfig.table.actions")})]})}),e.jsx(oe,{children:I.map(a=>e.jsxs(H,{children:[e.jsx(i,{children:e.jsx(n,{sx:{display:"flex",alignItems:"center",gap:1},children:a.name})}),e.jsx(i,{children:e.jsx(w,{label:O(a.type),size:"small",variant:"outlined",color:a.type==="feishu"?"primary":a.type==="dingtalk"?"info":"secondary"})}),e.jsx(i,{children:e.jsx(r,{variant:"body2",sx:{fontFamily:"monospace"},children:a.app_id})}),e.jsx(i,{children:e.jsx(r,{variant:"body2",sx:{fontFamily:"monospace"},children:a.approval_code||a.process_code||a.template_id||"-"})}),e.jsx(i,{align:"center",children:a.enabled?e.jsx(w,{icon:e.jsx(G,{}),label:l("common.enabled"),size:"small",color:"success"}):e.jsx(w,{icon:e.jsx($,{}),label:l("common.disabled"),size:"small",color:"default"})}),e.jsx(i,{align:"center",children:e.jsx(r,{variant:"body2",color:"text.secondary",children:R(a.created_at)})}),e.jsx(i,{align:"right",children:e.jsxs(n,{sx:{display:"flex",gap:.5,justifyContent:"flex-end"},children:[e.jsx(D,{title:l("common.edit"),children:e.jsx(q,{size:"small",color:"info",onClick:()=>F(a),children:e.jsx(K,{})})}),e.jsx(D,{title:l("common.delete"),children:e.jsx(q,{size:"small",color:"error",onClick:()=>P(a.id),children:e.jsx(Q,{})})})]})})]},a.id))})]})}),e.jsxs(X,{open:E,onClose:j,maxWidth:"md",fullWidth:!0,children:[e.jsx(Y,{children:l(d?"settings.approvalConfig.editConfig":"settings.approvalConfig.addConfig")}),e.jsx(Z,{children:e.jsxs(f,{spacing:3,sx:{mt:1},children:[e.jsx(r,{variant:"subtitle1",fontWeight:"600",children:l("settings.approvalConfig.basicConfig")}),e.jsx(o,{fullWidth:!0,label:l("settings.approvalConfig.configName"),value:t.name,onChange:a=>s({...t,name:a.target.value}),required:!0}),e.jsxs(ee,{fullWidth:!0,children:[e.jsx(ae,{children:l("settings.approvalConfig.platformType")}),e.jsxs(le,{value:t.type,label:l("settings.approvalConfig.platformType"),onChange:a=>L(a.target.value),children:[e.jsx(k,{value:"feishu",children:l("settings.approvalConfig.platforms.feishu")}),e.jsx(k,{value:"dingtalk",children:l("settings.approvalConfig.platforms.dingtalk")}),e.jsx(k,{value:"wechat",children:l("settings.approvalConfig.platforms.wechat")})]})]}),e.jsx(o,{fullWidth:!0,label:l("settings.approvalConfig.appId"),value:t.app_id,onChange:a=>s({...t,app_id:a.target.value}),required:!0,helperText:l("settings.approvalConfig.appIdHelper")}),e.jsx(o,{fullWidth:!0,label:l("settings.approvalConfig.appSecret"),type:"password",value:t.app_secret,onChange:a=>s({...t,app_secret:a.target.value}),required:!0,helperText:l(d?"settings.approvalConfig.appSecretHelperEdit":"settings.approvalConfig.appSecretHelper"),placeholder:d?l("settings.approvalConfig.appSecretPlaceholder"):""}),t.type==="feishu"&&e.jsx(o,{fullWidth:!0,label:l("settings.approvalConfig.approvalCode"),value:t.approval_code,onChange:a=>s({...t,approval_code:a.target.value}),required:!0,helperText:l("settings.approvalConfig.approvalCodeHelper")}),t.type==="dingtalk"&&e.jsx(o,{fullWidth:!0,label:l("settings.approvalConfig.processCode"),value:t.process_code,onChange:a=>s({...t,process_code:a.target.value}),required:!0,helperText:l("settings.approvalConfig.processCodeHelper")}),t.type==="wechat"&&e.jsx(o,{fullWidth:!0,label:l("settings.approvalConfig.templateId"),value:t.template_id,onChange:a=>s({...t,template_id:a.target.value}),required:!0,helperText:l("settings.approvalConfig.templateIdHelper")}),e.jsx(A,{}),e.jsxs(_,{children:[e.jsx(x,{expandIcon:e.jsx(v,{}),children:e.jsx(r,{variant:"subtitle1",fontWeight:"600",children:l("settings.approvalConfig.apiConfig.title")})}),e.jsx(m,{children:e.jsxs(f,{spacing:2,children:[e.jsx(o,{fullWidth:!0,label:l("settings.approvalConfig.apiConfig.apiBaseUrl"),value:t.api_base_url||"",onChange:a=>s({...t,api_base_url:a.target.value}),helperText:l("settings.approvalConfig.apiConfig.apiBaseUrlHelper"),placeholder:"https://open.larksuite.com/open-apis"}),e.jsxs(n,{children:[e.jsx(o,{fullWidth:!0,label:l("settings.approvalConfig.apiConfig.callbackUrl"),value:t.callback_url||"",onChange:a=>s({...t,callback_url:a.target.value}),helperText:l("settings.approvalConfig.apiConfig.callbackUrlHelper"),placeholder:"https://your-domain.com/api/approvals/callback/feishu"}),e.jsxs(n,{sx:{mt:1,display:"flex",gap:1},children:[e.jsx(g,{size:"small",variant:"outlined",onClick:()=>{const a=window.location.origin,u=`/api/approvals/callback/${t.type}`;s({...t,callback_url:a+u})},children:l("settings.approvalConfig.apiConfig.autoGenerateCallbackUrl")}),e.jsx(g,{size:"small",variant:"outlined",color:"info",onClick:()=>{t.callback_url&&(navigator.clipboard.writeText(t.callback_url),alert(l("settings.approvalConfig.apiConfig.callbackUrlCopied")))},disabled:!t.callback_url,children:l("settings.approvalConfig.apiConfig.copyCallbackUrl")})]})]})]})})]}),e.jsxs(_,{children:[e.jsx(x,{expandIcon:e.jsx(v,{}),children:e.jsx(r,{variant:"subtitle1",fontWeight:"600",children:l("settings.approvalConfig.approverConfig.title")})}),e.jsx(m,{children:e.jsx(f,{spacing:2,children:e.jsx(o,{fullWidth:!0,label:l("settings.approvalConfig.approverConfig.approverUserIds"),value:t.approver_user_ids||"",onChange:a=>s({...t,approver_user_ids:a.target.value}),helperText:l("settings.approvalConfig.approverConfig.approverUserIdsHelper"),placeholder:'["user1", "user2", "user3"]',multiline:!0,rows:3})})})]}),e.jsxs(_,{defaultExpanded:!0,children:[e.jsx(x,{expandIcon:e.jsx(v,{}),children:e.jsx(r,{variant:"subtitle1",fontWeight:"600",children:l("settings.approvalConfig.formFields")})}),e.jsx(m,{children:e.jsxs(f,{spacing:2,children:[e.jsx(y,{severity:"info",children:l("settings.approvalConfig.formFieldsHelper")}),e.jsx(o,{fullWidth:!0,label:l("settings.approvalConfig.formFieldsJson"),value:t.form_fields,onChange:a=>s({...t,form_fields:a.target.value}),multiline:!0,rows:12,required:!0,sx:{fontFamily:"monospace","& textarea":{fontFamily:"monospace",fontSize:"0.875rem"}}}),e.jsxs(_,{children:[e.jsx(x,{expandIcon:e.jsx(v,{}),children:e.jsx(r,{variant:"body2",color:"primary",children:l("settings.approvalConfig.viewExample")})}),e.jsx(m,{children:e.jsx(n,{sx:{bgcolor:"#f5f5f5",p:2,borderRadius:1,fontFamily:"monospace",fontSize:"0.75rem",overflow:"auto"},children:e.jsxs("pre",{style:{margin:0},children:[t.type==="feishu"&&p.feishu,t.type==="dingtalk"&&p.dingtalk,t.type==="wechat"&&p.wechat]})})})]})]})})]}),e.jsx(te,{control:e.jsx(pe,{checked:t.enabled,onChange:a=>s({...t,enabled:a.target.checked})}),label:l("settings.approvalConfig.enableConfig")})]})}),e.jsxs(se,{children:[e.jsx(g,{onClick:j,children:l("common.cancel")}),e.jsx(g,{variant:"contained",onClick:B,disabled:!t.name||!t.app_id||!t.app_secret,children:l(d?"common.save":"common.create")})]})]})]})})}export{he as default};
