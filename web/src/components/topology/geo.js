// Where a device is, for the map of the topology page: the region set on
// the device (a place key, in its network settings), and until one is set
// the place read from its tags and its name: a region code (JP, HK), an
// airport or city code (tyo, lax) or a city name in English or Chinese. A
// device with neither stays off the map and is listed under it.

// [key, latitude, longitude, English name, Chinese name, words]: codes match a whole word of the name,
// Chinese names anywhere in it. Cities come before their country, so
// "us-lax-01" is Los Angeles and "us-01" the country's marker.
const PLACES = [
  ['shanghai', 31.2, 121.5, 'Shanghai', '上海', 'sh sha pvg shanghai 上海'],
  ['beijing', 39.9, 116.4, 'Beijing', '北京', 'bj pek beijing 北京'],
  ['guangzhou', 23.1, 113.3, 'Guangzhou', '广州', 'gz can guangzhou 广州'],
  ['shenzhen', 22.5, 114.1, 'Shenzhen', '深圳', 'sz szx shenzhen 深圳'],
  ['hangzhou', 30.3, 120.2, 'Hangzhou', '杭州', 'hz hgh hangzhou 杭州'],
  ['chengdu', 30.7, 104.1, 'Chengdu', '成都', 'cd ctu chengdu 成都'],
  ['cn', 34.3, 108.9, 'China', '中国', 'cn china 中国 国内 境内'],
  ['hk', 22.3, 114.2, 'Hong Kong', '香港', 'hk hkg hongkong 香港'],
  ['tw', 25.0, 121.5, 'Taiwan', '台湾', 'tw tpe taiwan taipei 台湾 台北'],
  ['osaka', 34.7, 135.5, 'Osaka', '大阪', 'osa kix osaka 大阪'],
  ['jp', 35.7, 139.7, 'Tokyo', '东京', 'jp tyo nrt hnd japan tokyo 日本 东京'],
  ['kr', 37.6, 127.0, 'Seoul', '首尔', 'kr sel icn korea seoul 韩国 首尔'],
  ['sg', 1.35, 103.8, 'Singapore', '新加坡', 'sg sin singapore 新加坡'],
  ['my', 3.1, 101.7, 'Kuala Lumpur', '吉隆坡', 'kul malaysia 马来西亚'],
  ['th', 13.8, 100.5, 'Bangkok', '曼谷', 'th bkk thailand bangkok 泰国'],
  ['vn', 10.8, 106.7, 'Ho Chi Minh City', '胡志明市', 'vn sgn vietnam 越南'],
  ['ph', 14.6, 121.0, 'Manila', '马尼拉', 'ph mnl philippines manila 菲律宾'],
  ['jakarta', -6.2, 106.8, 'Jakarta', '雅加达', 'jkt cgk indonesia jakarta 印尼'],
  ['mumbai', 19.1, 72.9, 'Mumbai', '孟买', 'bom india mumbai 印度'],
  ['ae', 25.2, 55.3, 'Dubai', '迪拜', 'ae dxb dubai 迪拜'],
  ['tr', 41.0, 29.0, 'Istanbul', '伊斯坦布尔', 'tr ist turkey istanbul 土耳其'],
  ['ru', 55.8, 37.6, 'Moscow', '莫斯科', 'ru mow svo russia moscow 俄罗斯'],
  ['london', 51.5, -0.1, 'London', '伦敦', 'uk gb lon lhr london 英国 伦敦'],
  ['fr', 48.9, 2.4, 'Paris', '巴黎', 'fr par cdg france paris 法国 巴黎'],
  ['de', 50.1, 8.7, 'Frankfurt', '法兰克福', 'de fra germany frankfurt 德国 法兰克福'],
  ['nl', 52.4, 4.9, 'Amsterdam', '阿姆斯特丹', 'nl ams netherlands amsterdam 荷兰'],
  ['pl', 52.2, 21.0, 'Warsaw', '华沙', 'pl waw poland warsaw 波兰'],
  ['se', 59.3, 18.1, 'Stockholm', '斯德哥尔摩', 'se sto arn sweden stockholm 瑞典'],
  ['ch', 47.4, 8.5, 'Zurich', '苏黎世', 'ch zrh switzerland zurich 瑞士'],
  ['es', 40.4, -3.7, 'Madrid', '马德里', 'es mad spain madrid 西班牙'],
  ['milan', 45.5, 9.2, 'Milan', '米兰', 'mil mxp italy milan 意大利'],
  ['au', -33.9, 151.2, 'Sydney', '悉尼', 'au syd australia sydney 澳大利亚 悉尼'],
  ['losangeles', 34.1, -118.2, 'Los Angeles', '洛杉矶', 'lax losangeles 洛杉矶'],
  ['sanjose', 37.3, -121.9, 'San Jose', '圣何塞', 'sjc sfo sanjose siliconvalley 圣何塞 硅谷'],
  ['seattle', 47.6, -122.3, 'Seattle', '西雅图', 'sea seattle 西雅图'],
  ['newyork', 40.7, -74.0, 'New York', '纽约', 'nyc jfk ewr newyork 纽约'],
  ['chicago', 41.9, -87.6, 'Chicago', '芝加哥', 'chi ord chicago 芝加哥'],
  ['dallas', 32.8, -96.8, 'Dallas', '达拉斯', 'dfw dallas 达拉斯'],
  ['us', 39.0, -98.0, 'United States', '美国', 'us usa america 美国'],
  ['ca', 43.7, -79.4, 'Toronto', '多伦多', 'ca yyz canada toronto 加拿大'],
  ['br', -23.5, -46.6, 'São Paulo', '圣保罗', 'br gru brazil saopaulo 巴西']
].map(([key, lat, lon, en, zh, words]) => ({ key, lat, lon, en, zh, words: words.split(' ') }))

const ASCII_PLACE = new Map()
const CJK_PLACE = []
for (const place of PLACES) {
  for (const word of place.words) {
    if (/^[a-z]+$/.test(word)) ASCII_PLACE.set(word, place)
    else CJK_PLACE.push([word, place])
  }
}

// The words of a name: "ZouterHK" is zouter and hk, "us-lax-01" is us and lax.
function words(text) {
  return (text.match(/[A-Z]{2,}(?![a-z])|[A-Za-z][a-z]*/g) || []).map(word => word.toLowerCase())
}

function placeIn(text) {
  let best = null
  for (const word of words(text)) {
    const place = ASCII_PLACE.get(word)
    if (place && (!best || PLACES.indexOf(place) < PLACES.indexOf(best))) best = place
  }
  return best || CJK_PLACE.find(([word]) => text.includes(word))?.[1] || null
}

// The places a device's region can be, and the place of a region.
export const REGIONS = PLACES
export const placeOf = region => PLACES.find(place => place.key === region) || null

// locate answers the place ({ key, lat, lon, en, zh }) of a device from its
// name and its tags, the most specific one they name, or null.
export function locate({ name = '', tags = '' } = {}) {
  return placeIn(`${name || ''} ${tags || ''}`)
}

// The map: a Miller projection 1000 units wide that starts at 30°W, so
// Europe, Asia, the Pacific and the Americas read left to right and a line
// from Shanghai to Los Angeles crosses the Pacific.
export const MAP_WIDTH = 1000
export const MAP_HEIGHT = 470
const WEST = -30
const SCALE = MAP_WIDTH / (2 * Math.PI)
const miller = lat => 1.25 * Math.log(Math.tan(Math.PI / 4 + 0.4 * lat * Math.PI / 180))
const TOP = miller(80) * SCALE

export function project(lat, lon) {
  return { x: (((lon - WEST) % 360) + 360) % 360 / 360 * MAP_WIDTH, y: TOP - miller(lat) * SCALE }
}
