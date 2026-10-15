// 小程序入口配置：5 Tab（广场/圈子/猫立方/消息/我的，均为主包壳页）；
// 重功能（详情/开通/搜索）放分包按需加载。首屏 = 广场壳 + 单次聚合接口。
export default defineAppConfig({
  pages: [
    'pages/square/index',
    'pages/circle/index',
    'pages/cube/index',
    'pages/messages/index',
    'pages/mine/index',
  ],
  subPackages: [
    { root: 'pages/content', pages: ['detail', 'publish', 'mine'] },
    { root: 'pages/member', pages: ['open'] },
    { root: 'pages/search', pages: ['index'] },
    { root: 'pages/card', pages: ['edit'] },
    { root: 'pages/msg', pages: ['chat'] },
    { root: 'pages/social', pages: ['favorites', 'follows'] },
    { root: 'pages/auth', pages: ['enter'] },
    { root: 'pages/verify', pages: ['index'] },
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
      { pagePath: 'pages/square/index', text: '广场' },
      { pagePath: 'pages/circle/index', text: '圈子' },
      { pagePath: 'pages/cube/index', text: '猫立方' },
      { pagePath: 'pages/messages/index', text: '消息' },
      { pagePath: 'pages/mine/index', text: '我的' },
    ],
  },
})
