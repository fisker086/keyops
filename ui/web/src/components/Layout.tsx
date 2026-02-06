import React, { useState, useEffect, useRef, useCallback, useMemo } from 'react';
import { Outlet, useNavigate, useLocation } from 'react-router-dom';
import {
  Box,
  Drawer,
  AppBar,
  Toolbar,
  List,
  Typography,
  Divider,
  IconButton,
  ListItem,
  ListItemButton,
  ListItemIcon,
  ListItemText,
  CssBaseline,
  useTheme,
  Avatar,
  Badge,
  Menu,
  MenuItem,
  Tooltip,
  Collapse,
} from '@mui/material';
import {
  Menu as MenuIcon,
  ChevronLeft as ChevronLeftIcon,
  Computer as ComputerIcon,
  Logout as LogoutIcon,
  Person as PersonIcon,
  Brightness4 as Brightness4Icon,
  Brightness7 as Brightness7Icon,
  Language as LanguageIcon,
  ExpandLess,
  ExpandMore,
} from '@mui/icons-material';
import { useTerminal } from '../contexts/TerminalContext';
import { useSettings } from '../contexts/SettingsContext';
import { useTheme as useCustomTheme } from '../contexts/ThemeContext';
import { useTabs, Tab } from '../contexts/TabsContext';
import { useTranslation } from 'react-i18next';
import { getUserMenus, Menu as MenuType } from '../api/permission';
import { getIconByName } from '../utils/menuIcons';
import Watermark from './Watermark';
import TabsBar from './TabsBar';

const drawerWidth = 240; // Web 优化：减小侧边栏宽度

interface MenuItem {
  id: string;
  icon: React.ReactElement;
  path: string;
  title: string;
  children?: MenuItem[];
}

interface MenuGroup {
  id: string;
  title: string;
  icon?: React.ReactElement;
  items: MenuItem[];
}

export default function Layout() {
  const theme = useTheme();
  const { mode, toggleTheme } = useCustomTheme();
  const { t, i18n } = useTranslation();
  const navigate = useNavigate();
  const location = useLocation();
  const { sessions } = useTerminal();
  const { settings } = useSettings();
  const { tabs, addTab, removeTab, updateTabsClosable, cleanInvalidTabs, forceRemoveTab, getPendingRemovedTabId } = useTabs();
  const [open, setOpen] = useState(true);
  const [anchorEl, setAnchorEl] = useState<null | HTMLElement>(null);
  const [langAnchorEl, setLangAnchorEl] = useState<null | HTMLElement>(null);

  const user = JSON.parse(localStorage.getItem('user') || '{}');
  const username = user.username || 'User';
  const [menus, setMenus] = useState<MenuType[]>([]);
  const [loading, setLoading] = useState(true);
  const [openGroups, setOpenGroups] = useState<{ [key: string]: boolean }>({});
  const prevPathRef = useRef<string>(''); // 用于跟踪上一个路径

  // 从后端获取菜单（完全依赖后端返回的菜单数据，不再使用硬编码）
  // 使用 sessionStorage 缓存菜单数据，避免每次页面跳转都重新请求
  const fetchMenus = async (forceRefresh: boolean = false) => {
    // 检查缓存
    const cacheKey = 'user_menus_cache';
    const cacheTimestampKey = 'user_menus_cache_timestamp';
    const cacheExpiry = 10 * 60 * 1000; // 10分钟缓存过期时间
    
    if (!forceRefresh) {
      try {
        const cachedMenus = sessionStorage.getItem(cacheKey);
        const cachedTimestamp = sessionStorage.getItem(cacheTimestampKey);
        
        if (cachedMenus && cachedTimestamp) {
          const timestamp = parseInt(cachedTimestamp, 10);
          const now = Date.now();
          
          // 如果缓存未过期，直接使用缓存
          if (now - timestamp < cacheExpiry) {
            const menus = JSON.parse(cachedMenus);
            setMenus(menus);
            // 初始化展开状态：默认折叠所有分组
            const initialOpenState: { [key: string]: boolean } = {};
            menus.forEach((menu: MenuType) => {
              initialOpenState[menu.id] = false;
            });
            setOpenGroups(initialOpenState);
            setLoading(false);
            return;
          }
        }
      } catch (error) {
        console.warn('读取菜单缓存失败:', error);
        // 缓存读取失败，继续从后端获取
      }
    }
    
    try {
      setLoading(true);
      const response = await getUserMenus();
      if (response.data) {
        setMenus(response.data);
        // 缓存菜单数据
        try {
          sessionStorage.setItem(cacheKey, JSON.stringify(response.data));
          sessionStorage.setItem(cacheTimestampKey, Date.now().toString());
        } catch (error) {
          console.warn('保存菜单缓存失败:', error);
        }
        // 初始化展开状态：默认折叠所有分组
        const initialOpenState: { [key: string]: boolean } = {};
        response.data.forEach((menu) => {
          initialOpenState[menu.id] = false;
        });
        setOpenGroups(initialOpenState);
      }
    } catch (error) {
      console.error('获取菜单失败:', error);
      // 如果获取失败，尝试使用缓存（即使已过期）
      try {
        const cachedMenus = sessionStorage.getItem(cacheKey);
        if (cachedMenus) {
          const menus = JSON.parse(cachedMenus);
          setMenus(menus);
          const initialOpenState: { [key: string]: boolean } = {};
          menus.forEach((menu: MenuType) => {
            initialOpenState[menu.id] = false;
          });
          setOpenGroups(initialOpenState);
        } else {
          setMenus([]);
        }
      } catch (cacheError) {
        console.error('读取菜单缓存失败:', cacheError);
        setMenus([]);
      }
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    fetchMenus();

    // 监听菜单更新事件（强制刷新）
    const handleMenuUpdate = () => {
      fetchMenus(true); // 强制刷新，清除缓存
    };

    window.addEventListener('menuUpdated', handleMenuUpdate);
    
    // 监听菜单加载完成事件，清理无效的 tabs
    const handleMenusLoaded = (event: CustomEvent<MenuType[]>) => {
      cleanInvalidTabs(event.detail);
    };
    window.addEventListener('menusLoaded', handleMenusLoaded as EventListener);

    return () => {
      window.removeEventListener('menuUpdated', handleMenuUpdate);
      window.removeEventListener('menusLoaded', handleMenusLoaded as EventListener);
    };
  }, [cleanInvalidTabs]);

  // 查找菜单（根据路径）
  const findMenuByPath = useCallback((path: string, menuList: MenuType[]): MenuType | null => {
    for (const menu of menuList) {
      if (menu.path === path) {
        return menu;
      }
      if (menu.children && menu.children.length > 0) {
        const found = findMenuByPath(path, menu.children);
        if (found) return found;
      }
    }
    return null;
  }, []);

  // 判断路径是否应该激活某个菜单项（支持动态路由）
  const isPathActive = (menuPath: string, currentPath: string): boolean => {
    // 精确匹配
    if (menuPath === currentPath) {
      return true;
    }
    
    // 特殊处理：/fill-ticket-form 应该激活 /daily-workorders 菜单
    if (menuPath === '/daily-workorders' && currentPath.startsWith('/fill-ticket-form')) {
      return true;
    }
    
    // 特殊处理：/services/:id 应该激活 /services 菜单
    if (menuPath === '/services' && currentPath.startsWith('/services/') && currentPath !== '/services') {
      const pathParts = currentPath.split('/').filter(Boolean);
      // 如果路径是 /services/:id 格式（只有两个部分），则激活 /services 菜单
      if (pathParts.length === 2 && pathParts[0] === 'services') {
        return true;
      }
    }
    
    // 特殊处理：/clusters/:id/status 应该激活 /clusters 菜单
    if (menuPath === '/clusters' && currentPath.startsWith('/clusters/') && currentPath.endsWith('/status')) {
      return true;
    }
    
    // 特殊处理：/clusters/:id/permissions 应该激活 /cluster-permissions 菜单
    if (menuPath === '/cluster-permissions' && currentPath.startsWith('/clusters/') && currentPath.endsWith('/permissions')) {
      return true;
    }
    
    // 特殊处理：K8s 资源详情页应该激活对应的列表页菜单
    // 例如：/k8s/deployments/:clusterId/:namespace/:name 应该激活 /k8s/deployments
    if (menuPath === '/k8s/deployments' && currentPath.startsWith('/k8s/deployments/') && currentPath !== '/k8s/deployments') {
      // 检查是否是详情页格式：/k8s/deployments/:clusterId/:namespace/:name
      const pathParts = currentPath.split('/').filter(Boolean);
      if (pathParts.length >= 5 && pathParts[0] === 'k8s' && pathParts[1] === 'deployments') {
        return true;
      }
    }
    
    // 通用处理：K8s 资源详情页应该激活对应的列表页菜单
    // 匹配格式：/k8s/{resource}/:clusterId/:namespace/:name
    if (menuPath.startsWith('/k8s/') && currentPath.startsWith(menuPath + '/') && currentPath !== menuPath) {
      const pathParts = currentPath.split('/').filter(Boolean);
      const menuPathParts = menuPath.split('/').filter(Boolean);
      // 如果详情页路径以菜单路径开头，且路径段数更多，则认为是详情页
      if (pathParts.length > menuPathParts.length && pathParts.slice(0, menuPathParts.length).join('/') === menuPathParts.join('/')) {
        return true;
      }
    }
    
    // 特殊处理：/ticket/:id 根据来源路径激活对应的菜单
    if (currentPath.startsWith('/ticket/')) {
      const fromPath = (location.state as { from?: string } | null)?.from;
      if (fromPath) {
        // 如果来源路径匹配当前菜单路径，则激活
        return menuPath === fromPath;
      }
      // 如果没有来源路径，尝试从 referrer 判断
      // 检查是否从日常工单相关页面跳转过来
      const referrer = document.referrer;
      if (referrer) {
        // 如果 referrer 包含日常工单相关路径，激活日常工单菜单
        if (referrer.includes('/daily-workorders') || referrer.includes('/fill-ticket-form')) {
          return menuPath === '/daily-workorders';
        }
        // 如果 referrer 包含我的工单路径，激活我的工单菜单
        if (referrer.includes('/my-tickets')) {
          return menuPath === '/my-tickets';
        }
        // 如果 referrer 包含全部工单路径，激活全部工单菜单
        if (referrer.includes('/all-tickets')) {
          return menuPath === '/all-tickets';
        }
      }
      // 检查 sessionStorage 中是否有来源信息（用于页面刷新后保持状态）
      const storedFrom = sessionStorage.getItem('ticket_detail_from');
      if (storedFrom) {
        return menuPath === storedFrom;
      }
      // 默认激活"我的工单"（向后兼容）
      return menuPath === '/my-tickets';
    }
    
    return false;
  };

  // 当菜单加载完成后，更新所有 tabs 的标题和 closable 属性（刷新后恢复）
  useEffect(() => {
    if (menus.length > 0) {
      // 首先强制删除旧的 /dashboard 路径的 tab（即使它被标记为不可关闭）
      // 因为旧的 dashboard 菜单（menu-dashboard）已在 init.sql 中删除，路径为 /dashboard
      // 注意：只基于路径删除，不基于标题，避免误删未来可能添加的同名菜单
      tabs.forEach(tab => {
        if (tab.path === '/dashboard') {
          if (import.meta.env.DEV) {
            console.log('[Layout] Force removing old dashboard tab (path: /dashboard, even if not closable):', { id: tab.id, path: tab.path, title: tab.title });
          }
          // 使用 forceRemoveTab 强制删除，绕过 closable 检查
          forceRemoveTab('/dashboard');
        }
      });
      
      // 然后清理无效的 tabs（路径不在菜单中的）
      cleanInvalidTabs(menus);
      
      // 然后根据菜单配置更新所有 tabs 的 closable 属性
      // 这样确保从 localStorage 恢复的 tabs 的 closable 属性是正确的
      if (tabs.length > 0) {
        updateTabsClosable(menus);
      
        // 同时更新 tabs 的标题，确保标题与菜单配置一致
        // 这可以修复从 storage 恢复的 tab 标题不正确的问题
        // 注意：只在菜单首次加载时更新一次，避免无限循环
        const tabsToUpdate: Array<{ tab: Tab; menu: MenuType; correctTitle: string }> = [];
        const pathToTabMap = new Map<string, Tab>(); // 用于检测重复路径的 tab
        
        tabs.forEach(tab => {
          // 跳过已删除的 /dashboard tab
          if (tab.path === '/dashboard') {
            return;
          }
          
          const menu = findMenuByPath(tab.path, menus);
          if (menu) {
            const correctTitle = getMenuTitle(menu);
            // 检查是否有重复路径的 tab
            if (pathToTabMap.has(tab.path)) {
              // 如果已存在相同路径的 tab，保留标题正确的那个，删除重复的
              const existingTab = pathToTabMap.get(tab.path)!;
              if (existingTab.title === correctTitle && tab.title !== correctTitle) {
                // 保留已存在的正确标题的 tab，删除当前这个
                if (import.meta.env.DEV) {
                  console.log('[Layout] Removing duplicate tab with incorrect title:', { path: tab.path, title: tab.title });
                }
                removeTab(tab.id);
                return;
              } else if (tab.title === correctTitle && existingTab.title !== correctTitle) {
                // 当前 tab 标题正确，删除已存在的错误标题的 tab
                if (import.meta.env.DEV) {
                  console.log('[Layout] Removing duplicate tab with incorrect title:', { path: existingTab.path, title: existingTab.title });
                }
                removeTab(existingTab.id);
                pathToTabMap.set(tab.path, tab);
              } else {
                // 两个标题都不正确或都正确，保留第一个
                if (import.meta.env.DEV) {
                  console.log('[Layout] Removing duplicate tab:', { path: tab.path, title: tab.title });
                }
                removeTab(tab.id);
              }
            } else {
              pathToTabMap.set(tab.path, tab);
              // 如果标题不匹配，记录需要更新的 tab
              if (tab.title !== correctTitle) {
                tabsToUpdate.push({ tab, menu, correctTitle });
              }
            }
          }
        });
        
        // 批量更新 tabs 标题，避免多次触发 addTab
        if (tabsToUpdate.length > 0) {
          // 更新标签页标题
          if (import.meta.env.DEV) {
          console.log('[Layout] Updating tab titles:', tabsToUpdate.map(t => ({ 
            path: t.tab.path, 
            oldTitle: t.tab.title, 
            newTitle: t.correctTitle 
          })));
          }
          
          tabsToUpdate.forEach(({ menu, correctTitle }) => {
            // 创建一个带有正确标题的 menu 对象
            const menuWithCorrectTitle = {
              ...menu,
              meta: {
                ...menu.meta,
                title: correctTitle,
              },
            };
            addTab(menuWithCorrectTitle);
          });
        }
        
        // 注意：不在这里调用 addTab，因为上面的代码已经处理了标题更新
        // 下面的 useEffect 已经处理了路由变化时的 tab 添加
        // 这样可以避免两个 useEffect 互相触发导致无限循环
      }
    }
    // 移除 tabs 和 addTab 从依赖项，避免无限循环
    // 只在 menus 变化时执行一次
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [menus]);

  // 获取菜单标题（支持国际化）
  // 注意：后端返回的菜单数据中，meta.title 是数据库中的中文标题
  // 前端完全依赖 i18n 翻译，根据菜单的 name 字段查找翻译键，忽略 meta.title
  const getMenuTitle = useCallback((menu: MenuType): string => {
    // 先尝试 menu.groups.{name}（用于分组菜单，如 home, assets, bastion 等）
    const groupKey = `menu.groups.${menu.name}`;
    const groupTranslation = t(groupKey, { defaultValue: '' });
    if (groupTranslation && groupTranslation !== groupKey) {
      return groupTranslation;
    }
    
    // 再尝试 menu.{name}（用于普通菜单项，如 dashboard, terminal 等）
    const menuKey = `menu.${menu.name}`;
    const menuTranslation = t(menuKey, { defaultValue: '' });
    if (menuTranslation && menuTranslation !== menuKey) {
      return menuTranslation;
    }
    
    // 如果翻译不存在，使用 name 字段作为后备（而不是数据库中的 title）
    // 这样可以确保即使没有翻译，也能显示有意义的标识符
    return menu.name;
  }, [t]);

  // 监听语言变化，更新所有标签页的标题
  useEffect(() => {
    if (menus.length > 0 && tabs.length > 0) {
      const tabsToUpdate: Array<{ tab: Tab; menu: MenuType; correctTitle: string }> = [];
      
      tabs.forEach(tab => {
        const menu = findMenuByPath(tab.path, menus);
        if (menu) {
          const correctTitle = getMenuTitle(menu);
          // 如果标题不匹配，记录需要更新的 tab
          if (tab.title !== correctTitle) {
            tabsToUpdate.push({ tab, menu, correctTitle });
          }
        }
      });
      
      // 批量更新 tabs 标题
      if (tabsToUpdate.length > 0) {
        if (import.meta.env.DEV) {
          console.log('[Layout] Language changed, updating tab titles:', tabsToUpdate.map(t => ({ 
            path: t.tab.path, 
            oldTitle: t.tab.title, 
            newTitle: t.correctTitle 
          })));
        }
        
        tabsToUpdate.forEach(({ menu, correctTitle }) => {
          // 创建一个带有正确标题的 menu 对象
          const menuWithCorrectTitle = {
            ...menu,
            meta: {
              ...menu.meta,
              title: correctTitle,
            },
          };
          addTab(menuWithCorrectTitle);
        });
      }
    }
  }, [i18n.language, menus, tabs, getMenuTitle, findMenuByPath, addTab]);

  // 将后端菜单数据转换为前端需要的格式
  const convertMenuToMenuItem = useCallback((menu: MenuType): MenuItem => {
    // 图标从数据库配置中获取，如果没有配置则使用默认图标
    const icon = getIconByName(menu.meta?.icon);

    const item: MenuItem = {
      id: menu.id,
      icon,
      path: menu.path,
      title: getMenuTitle(menu),
    };

    // 如果有子菜单，递归转换（即使为空数组也保留，用于分组菜单）
    if (menu.children && menu.children.length > 0) {
      item.children = menu.children.map(child => convertMenuToMenuItem(child));
    } else if (menu.children && menu.children.length === 0) {
      // 如果 children 是空数组，也设置为空数组（用于分组菜单，即使没有子菜单也显示为目录）
      item.children = [];
    }

    return item;
  }, [getMenuTitle]);

  // 将后端返回的菜单树转换为前端分组格式
  // 后端返回的是树形结构，顶级菜单作为分组，子菜单作为菜单项
  // 完全依赖后端返回的菜单结构，不进行任何硬编码
  const convertMenusToGroups = useCallback((menus: MenuType[]): MenuGroup[] => {
    return menus
      .filter(menu => !menu.hidden) // 过滤隐藏的菜单
      .map(menu => {
        const group: MenuGroup = {
          id: menu.id,
          title: getMenuTitle(menu),
          icon: getIconByName(menu.meta?.icon), // 图标从数据库配置中获取，如果没有配置则使用默认图标
          items: [],
        };

        // 如果有子菜单，转换为items
        if (menu.children && menu.children.length > 0) {
          group.items = menu.children
            .filter(child => !child.hidden) // 过滤隐藏的子菜单
            .map(child => convertMenuToMenuItem(child));
        } else {
          // 如果没有子菜单，将自己作为单个item（单菜单项分组）
          group.items = [convertMenuToMenuItem(menu)];
        }

        return group;
      })
      .filter(group => group.items.length > 0); // 过滤掉没有菜单项的分组
  }, [getMenuTitle, convertMenuToMenuItem]);

  // 使用 useMemo 缓存 menuGroups，避免每次渲染都重新计算
  const menuGroups = useMemo(() => convertMenusToGroups(menus), [menus, convertMenusToGroups]);

  // 处理菜单点击，添加标签页
  const handleMenuClick = (path: string) => {
    // 跳过全屏页面（不在 Layout 内，不添加 tab）
    if (path === '/terminal' || path === '/template-editor') {
      // 保存当前路径到 sessionStorage，以便全屏页面返回时使用
      sessionStorage.setItem('terminal_previous_path', location.pathname);
      navigate(path);
      return;
    }
    
    // 权限管理页面（全屏，不在 Layout 内，不添加 tab）
    if (path === '/permissions') {
      // 保存当前路径到 sessionStorage，以便全屏页面返回时使用
      sessionStorage.setItem('permissions_previous_path', location.pathname);
      navigate(path);
      return;
    }
    
    // 查找菜单（包括隐藏的菜单）
    const findMenuIncludingHidden = (path: string, menuList: MenuType[]): MenuType | null => {
      for (const menu of menuList) {
        if (menu.path === path) {
          return menu;
        }
        if (menu.children && menu.children.length > 0) {
          const found = findMenuIncludingHidden(path, menu.children);
          if (found) return found;
        }
      }
      return null;
    };
    
    const menu = findMenuIncludingHidden(path, menus);
    if (menu && menu.component) {
      addTab(menu);
    }
    navigate(path);
  };

  // 监听路由变化，自动添加标签页和自动关闭标签页
  useEffect(() => {
    const currentPath = location.pathname;
    
    // 跳过静态资源路径（这些不应该被当作页面路由处理）
    // 静态资源路径通常有文件扩展名，或者是 /assets, /static 等目录
    if (currentPath.startsWith('/assets/') || 
        currentPath.startsWith('/static/') ||
        currentPath.match(/\.(js|css|png|jpg|jpeg|gif|ico|svg|woff|woff2|ttf|eot|map)$/i)) {
      // 静态资源路径，不处理 tab
      return;
    }
    
    // 跳过全屏页面（不在 Layout 内，不处理 tab）
    // 但是需要更新 prevPathRef，以便从全屏页面返回时能正确处理
    if (currentPath === '/terminal' || currentPath === '/template-editor' || currentPath.startsWith('/template-editor/')) {
      // 从全屏页面返回时，prevPathRef 可能还是全屏页面的路径
      // 这里不更新 prevPathRef，让它保持之前的值，这样返回时能正确添加 tab
      return;
    }
    
    // 如果上一个路径是全屏页面，清除它，避免影响当前路径的处理
    if (prevPathRef.current === '/terminal' || prevPathRef.current === '/template-editor' || 
        (prevPathRef.current && prevPathRef.current.startsWith('/template-editor/'))) {
      prevPathRef.current = '';
    }
    
    // 检查是否需要自动关闭上一个标签页
    if (prevPathRef.current && prevPathRef.current !== currentPath) {
      const prevMenu = findMenuByPath(prevPathRef.current, menus);
      if (prevMenu && prevMenu.meta?.closeTab) {
        // 如果上一个菜单配置了自动关闭，则关闭对应的标签页
        // 使用与 TabsContext 相同的逻辑生成 tabId
        const prevTabId = prevPathRef.current || 'home';
        removeTab(prevTabId);
      }
    }

    // 更新上一个路径（在添加新标签页之前更新，避免首次加载时误关闭）
    prevPathRef.current = currentPath;

    // 登录页和首页不添加标签页
    if (currentPath === '/login' || currentPath === '/') {
      return;
    }

    // 添加当前页面的标签页（如果菜单已加载）
    // 注意：只在菜单加载完成后才添加 tab，避免刷新时重复添加
    // 同时检查 tab 是否已存在，避免在 removeTab 后立即重新添加
    if (menus.length > 0) {
      const menu = findMenuByPath(currentPath, menus);
      // 如果找不到对应的菜单，说明这不是一个有效的页面路由，不处理
      if (!menu || !menu.component) {
        // 静默忽略，不输出日志（避免静态资源路径产生大量日志）
        return;
      }
      
      // 检查 tab 是否已存在（使用最新的 tabs 状态）
      const tabId = currentPath || 'home';
      const tabExists = tabs.some(tab => tab.id === tabId);
      
      // 检查是否有待处理的导航，如果有，并且当前路径是被移除的 tab 的路径，就不要添加
      const pendingRemovedTabId = getPendingRemovedTabId();
      const isPendingRemovedTab = pendingRemovedTabId === tabId;
      
      // 只在开发环境输出详细日志
      if (import.meta.env.DEV) {
        console.log('[Layout] Checking if should add tab:', {
          currentPath,
          tabId,
          tabExists,
          pendingRemovedTabId,
          isPendingRemovedTab,
          shouldAdd: !tabExists && !isPendingRemovedTab
        });
      }
      
      if (!tabExists && !isPendingRemovedTab) {
        // 如果 tab 不存在，添加 tab（addTab 会自动设置 activeTabId）
        if (import.meta.env.DEV) {
          console.log('[Layout] Adding tab for path:', currentPath);
        }
        addTab(menu);
      } else {
        if (import.meta.env.DEV) {
          if (tabExists) {
            console.log('[Layout] Tab already exists, skipping add');
          }
          if (isPendingRemovedTab) {
            console.log('[Layout] Tab is pending removal, skipping add');
          }
        }
      }
      // 注意：不要在这里主动同步 activeTabId
      // 原因：
      // 1. 当用户点击标签页时，setActiveTab 已经设置了 activeTabId 并导航了
      // 2. 当用户点击菜单时，addTab 已经设置了 activeTabId 并导航了
      // 3. activeTabId 的同步应该由 TabsContext.tsx 的 useEffect 来处理（页面刷新时）
      // 4. 如果在这里主动调用 setActiveTab，会与用户操作冲突，导致路径被错误地改回去
    }
  }, [location.pathname, menus, tabs, addTab, removeTab, findMenuByPath, getPendingRemovedTabId]);

  const handleGroupToggle = (groupId: string) => {
    setOpenGroups(prev => ({
      ...prev,
      [groupId]: !prev[groupId],
    }));
  };

  // 当路径变化时，自动展开包含当前路径的分组
  useEffect(() => {
    // 只在菜单组未展开时才展开，避免频繁更新状态导致菜单刷新
    menuGroups.forEach(group => {
      const hasActiveItem = group.items.some(item => {
        if (isPathActive(item.path, location.pathname)) {
          return true;
        }
        if (item.children) {
          return item.children.some(child => isPathActive(child.path, location.pathname));
        }
        return false;
      });
      if (hasActiveItem) {
        setOpenGroups(prev => {
          // 如果分组已经展开，直接返回 prev，避免不必要的状态更新
          if (prev[group.id] === true) {
            return prev;
          }
          // 只在需要展开时才更新状态
          return {
            ...prev,
            [group.id]: true,
          };
        });
      }
    });

    // 自动展开包含当前路径的二级菜单（三级菜单的父菜单）
    menuGroups.forEach(group => {
      group.items.forEach(item => {
        // 检查二级菜单是否有三级菜单匹配当前路径
        if (item.children && item.children.length > 0) {
          const hasActiveGrandchild = item.children.some(child => {
            // 检查三级菜单是否匹配当前路径
            if (isPathActive(child.path, location.pathname)) {
              return true;
            }
            // 检查是否有更深层的子菜单匹配
            if (child.children) {
              return child.children.some(grandchild => isPathActive(grandchild.path, location.pathname));
            }
            return false;
          });

          if (hasActiveGrandchild) {
            // 自动展开二级菜单
            setOpenGroups(prev => {
              const key = `${group.id}-${item.id}`;
              // 如果已经展开，直接返回 prev，避免不必要的状态更新
              if (prev[key] === true) {
                return prev;
              }
              return {
                ...prev,
                [key]: true,
              };
            });
          }
        }
      });
    });
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [location.pathname, menus]);

  const handleDrawerToggle = () => {
    setOpen(!open);
  };

  const handleMenuOpen = (event: React.MouseEvent<HTMLElement>) => {
    setAnchorEl(event.currentTarget);
  };

  const handleMenuClose = () => {
    setAnchorEl(null);
  };

  const handleLangMenuOpen = (event: React.MouseEvent<HTMLElement>) => {
    setLangAnchorEl(event.currentTarget);
  };

  const handleLangMenuClose = () => {
    setLangAnchorEl(null);
  };

  const handleLanguageChange = (lang: string) => {
    i18n.changeLanguage(lang);
    localStorage.setItem('language', lang);
    handleLangMenuClose();
  };

  const handleLogout = () => {
    // JWT token是无状态的，直接清除本地存储即可
    // 不需要调用后端API，避免token过期时显示"登录已过期"的提示
    localStorage.removeItem('token');
    localStorage.removeItem('user');
    
    // 清除菜单缓存
    sessionStorage.removeItem('user_menus_cache');
    sessionStorage.removeItem('user_menus_cache_timestamp');
    
    // 清除 tabs 缓存
    sessionStorage.removeItem('zjump_tabs_state');
    localStorage.removeItem('zjump_tabs_state');
    
    // 跳转到登录页
    navigate('/login');
  };

  // 获取用户全名或用户名作为水印内容
  const watermarkContent = user.fullName || user.username || 'KeyOps';

  return (
    <Box sx={{ display: 'flex', minHeight: '100vh', backgroundColor: theme.palette.background.default }}>
      <CssBaseline />
      {/* 水印组件 */}
      <Watermark 
        content={watermarkContent}
        enabled={settings?.showWatermark ?? false}
        gap={[180, 150]}
        zIndex={9999}
      />
      <AppBar
        position="fixed"
        elevation={0}
        sx={{
          zIndex: theme.zIndex.drawer + 1,
          backgroundColor: theme.palette.background.paper,
          borderBottom: `1px solid ${theme.palette.divider}`,
        }}
      >
        <Toolbar sx={{ justifyContent: 'space-between', py: 1 }}>
          <Box sx={{ display: 'flex', alignItems: 'center' }}>
            <IconButton
              aria-label="toggle drawer"
              onClick={handleDrawerToggle}
              edge="start"
              sx={{ 
                mr: 2,
                color: theme.palette.text.primary,
              }}
            >
              {open ? <ChevronLeftIcon /> : <MenuIcon />}
            </IconButton>
            <Avatar
              sx={{
                mr: 1.5,
                bgcolor: theme.palette.primary.main,
                width: 38,
                height: 38,
              }}
            >
              <ComputerIcon />
            </Avatar>
            <Box>
              <Typography 
                variant="h6" 
                noWrap 
                component="div" 
                sx={{ 
                  fontWeight: 600,
                  color: theme.palette.primary.main,
                  letterSpacing: '-0.3px',
                }}
              >
                {settings?.siteName?.split(' ')[0] || 'KeyOps'}
              </Typography>
              <Typography 
                variant="caption" 
                sx={{ 
                  color: theme.palette.text.secondary,
                  fontSize: '0.7rem',
                }}
              >
                {t('common.systemSubtitle')}
              </Typography>
            </Box>
          </Box>

          {/* 工具栏：语言切换、主题切换、用户菜单 */}
          <Box sx={{ display: 'flex', alignItems: 'center', gap: 1 }}>
            {/* 语言切换 */}
            <Tooltip title={t('settings.language')}>
              <IconButton
                onClick={handleLangMenuOpen}
                size="small"
                sx={{ color: theme.palette.text.primary }}
              >
                <LanguageIcon />
              </IconButton>
            </Tooltip>
            <Menu
              anchorEl={langAnchorEl}
              open={Boolean(langAnchorEl)}
              onClose={handleLangMenuClose}
            >
              <MenuItem 
                onClick={() => handleLanguageChange('zh')}
                selected={i18n.language === 'zh'}
              >
                🇨🇳 中文
              </MenuItem>
              <MenuItem 
                onClick={() => handleLanguageChange('en')}
                selected={i18n.language === 'en'}
              >
                🇺🇸 English
              </MenuItem>
            </Menu>

            {/* 主题切换 */}
            <Tooltip title={mode === 'dark' ? t('settings.lightMode') : t('settings.comfortMode')}>
              <IconButton
                onClick={toggleTheme}
                size="small"
                sx={{ color: theme.palette.text.primary }}
              >
                {mode === 'dark' ? <Brightness7Icon /> : <Brightness4Icon />}
              </IconButton>
            </Tooltip>

            <Divider orientation="vertical" flexItem sx={{ mx: 1 }} />

            <Typography variant="body2" sx={{ color: theme.palette.text.secondary, mr: 1 }}>
              {username}
            </Typography>
            <Tooltip title={t('common.actions')}>
              <IconButton
                onClick={handleMenuOpen}
                size="small"
                sx={{ color: theme.palette.text.primary }}
              >
                <Avatar sx={{ width: 32, height: 32, bgcolor: '#667eea' }}>
                  <PersonIcon fontSize="small" />
                </Avatar>
              </IconButton>
            </Tooltip>
            <Menu
              anchorEl={anchorEl}
              open={Boolean(anchorEl)}
              onClose={handleMenuClose}
              transformOrigin={{ horizontal: 'right', vertical: 'top' }}
              anchorOrigin={{ horizontal: 'right', vertical: 'bottom' }}
            >
              <MenuItem onClick={() => { handleMenuClick('/profile'); handleMenuClose(); }}>
                <ListItemIcon>
                  <PersonIcon fontSize="small" />
                </ListItemIcon>
                <ListItemText>{t('menu.profile')}</ListItemText>
              </MenuItem>
              <MenuItem onClick={handleLogout}>
                <ListItemIcon>
                  <LogoutIcon fontSize="small" />
                </ListItemIcon>
                <ListItemText>{t('login.logout')}</ListItemText>
              </MenuItem>
            </Menu>
          </Box>
        </Toolbar>
      </AppBar>
      <Drawer
        variant="persistent"
        open={open}
        sx={{
          width: drawerWidth,
          flexShrink: 0,
          '& .MuiDrawer-paper': {
            width: drawerWidth,
            boxSizing: 'border-box',
            backgroundColor: theme.palette.background.paper,
            borderRight: `1px solid ${theme.palette.divider}`,
          },
        }}
      >
        <Toolbar />
        <Box sx={{ overflow: 'auto', mt: 3, px: 2 }}>
          {loading ? (
            <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
              <Typography variant="body2" color="text.secondary">
                {t('common.loading')}
              </Typography>
            </Box>
          ) : menuGroups.length === 0 ? (
            <Box sx={{ display: 'flex', justifyContent: 'center', py: 4 }}>
              <Typography variant="body2" color="text.secondary">
                暂无菜单权限
              </Typography>
            </Box>
          ) : (
          <List>
              {menuGroups.map((group) => {
                // 如果是单菜单项分组，直接渲染菜单项（不显示分组标题）
                // 但是首页分组（menu-home）、配置管理（menu-config）和数据库管理（menu-dms）
                // 即使只有1个子菜单，也要显示分组标题（目录形式）
                if (group.id !== 'menu-home' && group.id !== 'menu-config' && group.id !== 'menu-dms' && group.items.length === 1 && (!group.items[0].children || group.items[0].children.length === 0)) {
                  const item = group.items[0];
                  return (
                    <ListItem key={item.id} disablePadding sx={{ mb: 0.5 }}>
                <ListItemButton
                  selected={isPathActive(item.path, location.pathname)}
                  onClick={() => handleMenuClick(item.path)}
                  sx={{
                    borderRadius: 1.5,
                    py: 1.2,
                    px: 2,
                    '&.Mui-selected': {
                      backgroundColor: mode === 'dark' ? 'rgba(91, 124, 153, 0.15)' : '#ebf4ff',
                      color: theme.palette.primary.main,
                      '&:hover': {
                        backgroundColor: mode === 'dark' ? 'rgba(91, 124, 153, 0.22)' : '#dbeafe',
                      },
                    },
                    '&:hover': {
                      backgroundColor: mode === 'dark' ? 'rgba(139, 157, 119, 0.08)' : '#f7fafc',
                    },
                  }}
                >
                  <ListItemIcon
                    sx={{
                      color: isPathActive(item.path, location.pathname)
                        ? theme.palette.primary.main 
                        : theme.palette.text.secondary,
                      minWidth: 42,
                    }}
                  >
                    {item.path === '/terminal' && sessions.length > 0 ? (
                      <Badge 
                        badgeContent={sessions.length} 
                        color="primary"
                        max={99}
                      >
                        {item.icon}
                      </Badge>
                    ) : (
                      item.icon
                    )}
                  </ListItemIcon>
                  <ListItemText 
                          primary={item.title}
                    primaryTypographyProps={{
                      fontSize: '0.9rem',
                      fontWeight: isPathActive(item.path, location.pathname) ? 600 : 500,
                    }}
                  />
                </ListItemButton>
              </ListItem>
                  );
                }

                // 多菜单项分组，渲染可折叠分组
                const isOpen = openGroups[group.id] ?? true;
                const hasActiveItem = group.items.some(item => {
                  if (isPathActive(item.path, location.pathname)) {
                    return true;
                  }
                  if (item.children) {
                    return item.children.some(child => isPathActive(child.path, location.pathname));
                  }
                  return false;
                });

                return (
                  <React.Fragment key={group.id}>
                    <ListItem disablePadding sx={{ mb: 0.5 }}>
                      <ListItemButton
                        onClick={() => handleGroupToggle(group.id)}
                        sx={{
                          borderRadius: 1.5,
                          py: 1.2,
                          px: 2,
                          backgroundColor: hasActiveItem && isOpen
                            ? (mode === 'dark' ? 'rgba(91, 124, 153, 0.1)' : '#f0f7ff')
                            : 'transparent',
                          '&:hover': {
                            backgroundColor: mode === 'dark' ? 'rgba(139, 157, 119, 0.08)' : '#f7fafc',
                          },
                        }}
                      >
                        {group.icon && (
                          <ListItemIcon
                            sx={{
                              color: theme.palette.text.secondary,
                              minWidth: 42,
                            }}
                          >
                            {group.icon}
                          </ListItemIcon>
                        )}
                        <ListItemText 
                          primary={group.title}
                          primaryTypographyProps={{
                            fontSize: '0.85rem',
                            fontWeight: 600,
                            color: theme.palette.text.secondary,
                            textTransform: 'uppercase',
                            letterSpacing: '0.5px',
                          }}
                        />
                        {isOpen ? <ExpandLess /> : <ExpandMore />}
                      </ListItemButton>
                    </ListItem>
                    <Collapse in={isOpen} timeout="auto" unmountOnExit>
                      <List component="div" disablePadding>
                        {group.items.map((item) => {
                          // 递归渲染菜单项（支持多级嵌套）
                          const renderMenuItem = (menuItem: MenuItem, level: number = 0): React.ReactNode => {
                            const hasChildren = menuItem.children && menuItem.children.length > 0;
                            // 如果 path 为空字符串，即使没有 children，也视为分组菜单（目录）
                            const isGroupMenu = !menuItem.path || menuItem.path === '';
                            const itemIsOpen = openGroups[`${group.id}-${menuItem.id}`] ?? false;
                            const hasActiveChild = hasChildren && menuItem.children?.some(child => 
                              isPathActive(child.path, location.pathname) || 
                              (child.children && child.children.some(grandchild => isPathActive(grandchild.path, location.pathname)))
                            );
                            // 对于分组菜单（path为空），不应该被标记为选中，只有实际匹配路径的菜单项才应该被选中
                            const isSelected = !!(menuItem.path && isPathActive(menuItem.path, location.pathname));

                            if (hasChildren || isGroupMenu) {
                              // 有子菜单，渲染为可折叠项
                              return (
                                <React.Fragment key={menuItem.id}>
                                  <ListItem disablePadding sx={{ mb: 0.5, pl: 2 + level * 2 }}>
                                    <ListItemButton
                                      onClick={() => {
                                        // 如果是分组菜单但没有子菜单，不处理点击（避免展开空的分组）
                                        if (isGroupMenu && !hasChildren) {
                                          return;
                                        }
                                        setOpenGroups(prev => ({
                                          ...prev,
                                          [`${group.id}-${menuItem.id}`]: !prev[`${group.id}-${menuItem.id}`],
                                        }));
                                      }}
                                      sx={{
                                        borderRadius: 1.5,
                                        py: 1.2,
                                        px: 2,
                                        backgroundColor: (hasActiveChild && itemIsOpen) || isSelected
                                          ? (mode === 'dark' ? 'rgba(91, 124, 153, 0.1)' : '#f0f7ff')
                                          : 'transparent',
                                        '&:hover': {
                                          backgroundColor: mode === 'dark' ? 'rgba(139, 157, 119, 0.08)' : '#f7fafc',
                                        },
                                      }}
                                    >
                                      <ListItemIcon
                                        sx={{
                                          color: isSelected 
                                            ? theme.palette.primary.main 
                                            : theme.palette.text.secondary,
                                          minWidth: 42,
                                        }}
                                      >
                                        {menuItem.path === '/terminal' && sessions.length > 0 ? (
                                          <Badge 
                                            badgeContent={sessions.length} 
                                            color="primary"
                                            max={99}
                                          >
                                            {menuItem.icon}
                                          </Badge>
                                        ) : (
                                          menuItem.icon
                                        )}
                                      </ListItemIcon>
                                      <ListItemText 
                                        primary={menuItem.title}
                                        primaryTypographyProps={{
                                          fontSize: '0.9rem',
                                          fontWeight: isSelected ? 600 : 500,
                                        }}
                                      />
                                      {(hasChildren || isGroupMenu) && (itemIsOpen ? <ExpandLess /> : <ExpandMore />)}
                                    </ListItemButton>
                                  </ListItem>
                                  <Collapse in={itemIsOpen} timeout="auto" unmountOnExit>
                                    <List component="div" disablePadding>
                                      {menuItem.children?.map(child => renderMenuItem(child, level + 1))}
                                    </List>
                                  </Collapse>
                                </React.Fragment>
                              );
                            } else {
                              // 没有子菜单，渲染为普通菜单项
                              return (
                                <ListItem key={menuItem.id} disablePadding sx={{ mb: 0.5, pl: 2 + level * 2 }}>
                                  <ListItemButton
                                    selected={isSelected}
                                    onClick={() => handleMenuClick(menuItem.path)}
                                    sx={{
                                      borderRadius: 1.5,
                                      py: 1.2,
                                      px: 2,
                                      '&.Mui-selected': {
                                        backgroundColor: mode === 'dark' ? 'rgba(91, 124, 153, 0.15)' : '#ebf4ff',
                                        color: theme.palette.primary.main,
                                        '&:hover': {
                                          backgroundColor: mode === 'dark' ? 'rgba(91, 124, 153, 0.22)' : '#dbeafe',
                                        },
                                      },
                                      '&:hover': {
                                        backgroundColor: mode === 'dark' ? 'rgba(139, 157, 119, 0.08)' : '#f7fafc',
                                      },
                                    }}
                                  >
                                    <ListItemIcon
                                      sx={{
                                        color: isSelected 
                                          ? theme.palette.primary.main 
                                          : theme.palette.text.secondary,
                                        minWidth: 42,
                                      }}
                                    >
                                      {menuItem.path === '/terminal' && sessions.length > 0 ? (
                                        <Badge 
                                          badgeContent={sessions.length} 
                                          color="primary"
                                          max={99}
                                        >
                                          {menuItem.icon}
                                        </Badge>
                                      ) : (
                                        menuItem.icon
                                      )}
                                    </ListItemIcon>
                                    <ListItemText 
                                      primary={menuItem.title}
                                      primaryTypographyProps={{
                                        fontSize: '0.9rem',
                                        fontWeight: isSelected ? 600 : 500,
                                      }}
                                    />
                                  </ListItemButton>
                                </ListItem>
                              );
                            }
                          };

                          return renderMenuItem(item);
                        })}
                      </List>
                    </Collapse>
                  </React.Fragment>
                );
              })}
          </List>
          )}
          <Divider sx={{ my: 3 }} />
          <Box sx={{ px: 2, pb: 2 }}>
            <Typography variant="caption" color="text.secondary" sx={{ fontSize: '0.7rem' }}>
              © 2026 KeyOps v0.2.0
            </Typography>
          </Box>
        </Box>
      </Drawer>
      <Box
        component="main"
        sx={{
          flexGrow: 1,
          display: 'flex',
          flexDirection: 'column',
          width: { sm: `calc(100% - ${open ? drawerWidth : 0}px)` },
          ml: open ? 0 : `-${drawerWidth}px`,
          transition: theme.transitions.create(['margin', 'width'], {
            easing: theme.transitions.easing.sharp,
            duration: theme.transitions.duration.leavingScreen,
          }),
          minHeight: '100vh',
          backgroundColor: theme.palette.background.default,
        }}
      >
        <Toolbar /> {/* 为 AppBar 留出空间 */}
        {/* 多标签页栏 */}
        <TabsBar />
        {/* 页面内容区域 */}
        <Box
          sx={{
            flex: 1,
            p: 3,
            overflow: 'auto',
          }}
        >
          <Outlet />
        </Box>
      </Box>
    </Box>
  );
}

