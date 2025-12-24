import request from '@/utils/request'

// 获取用户资料
export function getProfile() {
  return request({
    url: '/user/profile',
    method: 'get'
  })
}

// 获取用户仪表盘数据
export function getDashboard() {
  return request({
    url: '/user/dashboard',
    method: 'get'
  })
}

// 获取用户订阅详情
export function getSubscription(refresh = false) {
  return request({
    url: '/user/subscription',
    method: 'get',
    params: refresh ? { refresh: 'true' } : {}
  })
}
