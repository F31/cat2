// 小程序入口配置：4 Tab（发现/猫立方/消息/我的，均为主包壳页）；
// 发现 = 广场 + 圈子双频道；重功能（详情/开通/搜索）放分包按需加载。
export default defineAppConfig({
  pages: [
    'pages/discover/index',
    'pages/home-circle/index',
    'pages/home-cube/index',
    'pages/messages/index',
    'pages/mine/index',
    'pages/mine/activities',
    'pages/settings/index',
    'pages/settings/agreement',
    'pages/subaccounts/index',
  ],
  subPackages: [
    { root: 'pages/content', pages: ['detail', 'publish', 'mine', 'signups'] },
    { root: 'pages/member', pages: ['open'] },
    { root: 'pages/search', pages: ['index'] },
    { root: 'pages/card', pages: ['edit'] },
    { root: 'pages/msg', pages: ['chat', 'notices'] },
    { root: 'pages/social', pages: ['favorites', 'follows'] },
    { root: 'pages/auth', pages: ['enter'] },
    { root: 'pages/verify', pages: ['index'] },
    { root: 'pages/cube', pages: ['create', 'detail', 'edit', 'project', 'task', 'report', 'plan'] },
    { root: 'pages/circle', pages: ['create', 'detail', 'applications', 'publish', 'settings', 'mounts'] },
    { root: 'pages/support', pages: ['index'] },
    { root: 'pages/channel', pages: ['index', 'join'] },
  ],
  window: {
    backgroundTextStyle: 'light',
    navigationBarBackgroundColor: '#ffffff',
    navigationBarTitleText: '猫平方',
    navigationBarTextStyle: 'black',
  },
  tabBar: {
    color: '#8a8a8a',
    selectedColor: '#d4411e',
    backgroundColor: '#ffffff',
    borderStyle: 'black',
    list: [
      {
        pagePath: 'pages/discover/index',
        text: '发现',
        iconPath: 'assets/tabbar/square.png',
        selectedIconPath: 'assets/tabbar/square-active.png',
      },
      {
        pagePath: 'pages/home-cube/index',
        text: '猫立方',
        iconPath: 'assets/tabbar/cube.png',
        selectedIconPath: 'assets/tabbar/cube-active.png',
      },
      {
        pagePath: 'pages/messages/index',
        text: '消息',
        iconPath: 'assets/tabbar/message.png',
        selectedIconPath: 'assets/tabbar/message-active.png',
      },
      {
        pagePath: 'pages/mine/index',
        text: '我的',
        iconPath: 'assets/tabbar/mine.png',
        selectedIconPath: 'assets/tabbar/mine-active.png',
      },
    ],
  },
})
