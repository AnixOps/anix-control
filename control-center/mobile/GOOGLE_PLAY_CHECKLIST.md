# Google Play Store 上架检查清单

## 构建状态 ✅

| 项目 | 状态 | 说明 |
|------|------|------|
| AAB 文件 | ✅ | app-release.aab (44.7 MB) |
| APK 文件 | ✅ | app-release.apk (51.6 MB) |
| 签名配置 | ✅ | keystore + key.properties |

## 必须项 ✅

| 项目 | 状态 | 说明 |
|------|------|------|
| build.gradle | ✅ | App级和项目级配置 |
| settings.gradle | ✅ | 项目设置 |
| gradle.properties | ✅ | Gradle属性 |
| proguard-rules.pro | ✅ | ProGuard规则 |
| AndroidManifest.xml | ✅ | 应用清单 |
| 网络安全配置 | ✅ | network_security_config.xml |
| 应用图标 (所有尺寸) | ✅ | mipmap-* 文件夹 |
| 通知图标 | ✅ | ic_notification.xml |
| file_paths.xml | ✅ | FileProvider配置 |
| MainActivity.kt | ✅ | 主活动 |
| colors.xml | ✅ | 颜色资源 |
| styles.xml | ✅ | 样式资源 |

## Firebase配置 🔥

| 项目 | 状态 | 说明 |
|------|------|------|
| google-services.json | ✅ | 已由用户添加 |

## 隐私与法律 📜

| 项目 | 状态 | 说明 |
|------|------|------|
| Privacy Policy页面 | ✅ | privacy_policy_page.dart |
| Terms of Service页面 | ✅ | terms_of_service_page.dart |
| Privacy Policy HTML | ✅ | store/privacy.html |
| Terms of Service HTML | ✅ | store/terms.html |
| 隐私政策URL | ⚠️ | 需要托管: https://anixops.com/privacy |
| 服务条款URL | ⚠️ | 需要托管: https://anixops.com/terms |

## 商店列表 📱

| 项目 | 状态 | 说明 |
|------|------|------|
| 商店材料文件 | ✅ | STORE_LISTING.md |
| 应用标题 | ✅ | AnixOps Control Center |
| 简短描述 | ✅ | 80字符以内 |
| 完整描述 | ✅ | 4000字符以内 |
| 应用截图 | ⚠️ | 需要至少2张 (1080x1920) |
| 宣传图 | ⚠️ | 需要 1024x500 |
| 应用图标 | ✅ | 512x512 |
| 分类 | ✅ | Tools / Productivity |

## 内容分级 🎯

| 项目 | 状态 | 说明 |
|------|------|------|
| 内容分级问卷 | ✅ | 答案在 STORE_LISTING.md |
| 预期评级 | ✅ | Everyone (E) |

## 构建命令 🔨

```bash
# 生成 App Bundle (用于 Play Store)
cd mobile && flutter build appbundle --release

# 生成 APK (用于测试)
cd mobile && flutter build apk --release
```

## 上架前检查 ✨

1. [x] 在Firebase创建项目并获取google-services.json
2. [x] 创建签名密钥库 (.jks文件)
3. [x] 配置key.properties
4. [x] 构建签名的App Bundle
5. [ ] 在Play Console创建应用
6. [ ] 托管隐私政策和服务条款页面
7. [ ] 制作商店截图 (至少2张)
8. [ ] 制作宣传图 (1024x500)
9. [ ] 填写商店列表信息
10. [ ] 上传App Bundle
11. [ ] 完成内容分级问卷
12. [ ] 设置定价和分发区域
13. [ ] 提交审核

## 注意事项 ⚠️

- targetSdkVersion: 34 (Android 14)
- minSdkVersion: 21 (Android 5.0)
- 使用App Bundle格式 (.aab)
- 必须有隐私政策URL
- 需要遵守Google Play政策