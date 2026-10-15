import { defineConfig } from '@tarojs/cli'
import devConfig from './dev'
import prodConfig from './prod'

// Taro 编译配置（weapp）。designWidth 750（rpx）。
export default defineConfig(async (merge) => {
  const baseConfig = {
    projectName: 'catcube-miniapp',
    date: '2025-1-1',
    designWidth: 750,
    deviceRatio: { 640: 2.34 / 2, 750: 1, 828: 1.81 / 2 },
    sourceRoot: 'src',
    outputRoot: 'dist',
    plugins: [],
    defineConstants: {},
    copy: { patterns: [], options: {} },
    framework: 'react',
    compiler: 'webpack5',
    cache: { enable: false },
    mini: {
      // 编译 workspace 内 TS 源码包（@catcube/* 经 pnpm 软链，默认被当作 node_modules 跳过）
      compile: {
        include: [(filename: string) => filename.includes('/packages/')],
      },
      postcss: {
        pxtransform: { enable: true, config: {} },
        url: { enable: true, config: { limit: 1024 } },
        cssModules: { enable: false },
      },
    },
    h5: {},
  }
  if (process.env.NODE_ENV === 'development') {
    return merge({}, baseConfig, devConfig)
  }
  return merge({}, baseConfig, prodConfig)
})
