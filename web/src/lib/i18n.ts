import { useEffect } from 'react';
import { useAtom } from 'jotai';
import { atomWithStorage } from 'jotai/utils';

export type Locale = 'en' | 'de' | 'zh';

export const localeAtom = atomWithStorage<Locale>('mvs.locale', 'en');

const en = {
  'nav.library': 'Library',
  'nav.home': 'Home',
  'nav.movies': 'Movies',
  'nav.tv': 'TV Shows',
  'nav.homeVideos': 'Home Videos',
  'nav.playlists': 'Playlists',
  'nav.manage': 'Manage',
  'nav.libraries': 'Libraries',
  'nav.users': 'Users',
  'nav.activity': 'Activity',
  'nav.tasks': 'Tasks',
  'nav.settings': 'Settings',
  'nav.signOut': 'Sign out',
  'nav.changePassword': 'Change password',
  'nav.language': 'Language',
  'auth.signIn': 'Sign in',
  'auth.signInSubtitle': 'Use your MyVideoStation account',
  'auth.username': 'Username',
  'auth.password': 'Password',
  'auth.signingIn': 'Signing in…',
  'auth.setupTitle': 'Set up MyVideoStation',
  'auth.setupSubtitle': 'Create the first admin account. This prompt appears once.',
  'auth.displayName': 'Display name',
  'auth.confirmPassword': 'Confirm password',
  'auth.createAdmin': 'Create admin account',
  'auth.creating': 'Creating…',
  'auth.passwordHint': 'At least 8 characters.',
  'users.title': 'Users',
  'users.subtitle': 'Admins manage libraries, metadata and settings. Users can browse, play and keep their own watch state.',
  'users.add': 'Add user',
  'activity.title': 'Activity',
  'activity.subtitle': 'Sign-ins, account changes, library operations and metadata edits.',
  'activity.refresh': 'Refresh',
};

const de: Record<keyof typeof en, string> = {
  'nav.library': 'Bibliothek',
  'nav.home': 'Start',
  'nav.movies': 'Filme',
  'nav.tv': 'Serien',
  'nav.homeVideos': 'Heimvideos',
  'nav.playlists': 'Playlists',
  'nav.manage': 'Verwaltung',
  'nav.libraries': 'Bibliotheken',
  'nav.users': 'Benutzer',
  'nav.activity': 'Aktivität',
  'nav.tasks': 'Aufgaben',
  'nav.settings': 'Einstellungen',
  'nav.signOut': 'Abmelden',
  'nav.changePassword': 'Passwort ändern',
  'nav.language': 'Sprache',
  'auth.signIn': 'Anmelden',
  'auth.signInSubtitle': 'Mit Ihrem MyVideoStation-Konto anmelden',
  'auth.username': 'Benutzername',
  'auth.password': 'Passwort',
  'auth.signingIn': 'Anmeldung läuft…',
  'auth.setupTitle': 'MyVideoStation einrichten',
  'auth.setupSubtitle': 'Erstellen Sie das erste Administratorkonto. Diese Aufforderung erscheint einmal.',
  'auth.displayName': 'Anzeigename',
  'auth.confirmPassword': 'Passwort bestätigen',
  'auth.createAdmin': 'Administratorkonto erstellen',
  'auth.creating': 'Wird erstellt…',
  'auth.passwordHint': 'Mindestens 8 Zeichen.',
  'users.title': 'Benutzer',
  'users.subtitle': 'Administratoren verwalten Bibliotheken, Metadaten und Einstellungen. Benutzer können stöbern, abspielen und ihren eigenen Wiedergabestatus behalten.',
  'users.add': 'Benutzer hinzufügen',
  'activity.title': 'Aktivität',
  'activity.subtitle': 'Anmeldungen, Kontenänderungen, Bibliotheksvorgänge und Metadatenbearbeitungen.',
  'activity.refresh': 'Aktualisieren',
};

const zh: Record<keyof typeof en, string> = {
  'nav.library': '媒体库',
  'nav.home': '首页',
  'nav.movies': '电影',
  'nav.tv': '电视剧',
  'nav.homeVideos': '家庭视频',
  'nav.playlists': '播放列表',
  'nav.manage': '管理',
  'nav.libraries': '媒体库',
  'nav.users': '用户',
  'nav.activity': '活动',
  'nav.tasks': '任务',
  'nav.settings': '设置',
  'nav.signOut': '退出登录',
  'nav.changePassword': '修改密码',
  'nav.language': '语言',
  'auth.signIn': '登录',
  'auth.signInSubtitle': '使用您的 MyVideoStation 帐户',
  'auth.username': '用户名',
  'auth.password': '密码',
  'auth.signingIn': '登录中…',
  'auth.setupTitle': '设置 MyVideoStation',
  'auth.setupSubtitle': '创建第一个管理员帐户。此提示仅显示一次。',
  'auth.displayName': '显示名称',
  'auth.confirmPassword': '确认密码',
  'auth.createAdmin': '创建管理员帐户',
  'auth.creating': '创建中…',
  'auth.passwordHint': '至少 8 个字符。',
  'users.title': '用户',
  'users.subtitle': '管理员管理媒体库、元数据和设置。用户可以浏览、播放并保留自己的观看状态。',
  'users.add': '添加用户',
  'activity.title': '活动',
  'activity.subtitle': '登录、帐户更改、媒体库操作和元数据编辑。',
  'activity.refresh': '刷新',
};

export type I18nKey = keyof typeof en;
const dictionaries: Record<Locale, Record<I18nKey, string>> = { en, de, zh };

export function useI18n() {
  const [locale, setLocale] = useAtom(localeAtom);
  useEffect(() => {
    document.documentElement.lang = locale === 'zh' ? 'zh-CN' : locale;
  }, [locale]);
  const t = (key: I18nKey) => dictionaries[locale]?.[key] ?? en[key];
  return { locale, setLocale, t };
}
