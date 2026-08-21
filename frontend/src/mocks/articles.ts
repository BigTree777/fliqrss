import type { Article } from '../types/article'

export const articles: Article[] = [
  {
    id: 'future-interface',
    source: 'Orbit Journal',
    sourceInitials: 'OJ',
    publishedAt: '12分前',
    readTime: 4,
    title: '画面のないコンピューターが, 暮らしの輪郭を変えはじめた',
    summary:
      '身につけるAIデバイスと音声インターフェース. 次のコンピューティング体験をつくる小さな変化を追う.',
    body: [
      'スマートフォンを取り出さずに情報へ触れる体験が, 少しずつ日常へ入りはじめています. 音声と小型センサーを組み合わせたデバイスは, 必要な瞬間だけ静かに情報を差し出します.',
      '重要なのは画面が消えることではなく, 人と情報の距離が変わることです. 通知を増やすのではなく, 本当に必要な情報を選ぶ設計が求められています.',
    ],
    visualLabel: 'NEW INTERFACE',
    visualTheme: 'cobalt',
  },
  {
    id: 'small-city-business',
    source: 'Business Field',
    sourceInitials: 'BF',
    publishedAt: '28分前',
    readTime: 6,
    title: '小さな都市から生まれる, 新しい働き方のネットワーク',
    summary:
      '場所よりも関係性を選ぶチームが増えている. 地域に根ざしながら世界と働く人々の現在地.',
    body: [
      '都市への集中を前提にしないチームづくりが広がっています. 小さな拠点を行き来しながら, 得意分野の異なる人がプロジェクトごとに集まります.',
      'オンラインだけでは生まれにくい偶然の会話を残しながら, 移動の負担を減らす. その両立が新しい組織設計の焦点です.',
    ],
    visualLabel: 'LOCAL / GLOBAL',
    visualTheme: 'coral',
  },
  {
    id: 'night-museum',
    source: 'Nook Magazine',
    sourceInitials: 'NM',
    publishedAt: '1時間前',
    readTime: 3,
    title: '夜のミュージアムで出会う, もうひとつの街の表情',
    summary:
      '閉館時間を越えて開かれる展示と対話. 静かな夜に文化施設が担う役割を考える.',
    body: [
      '日中とは違う速度で作品と向き合える夜間開館が注目されています. 仕事帰りの人や, 混雑を避けたい人にとって新しい居場所になっています.',
      '展示を見るだけでなく, 小さな対話や音楽が同じ空間に重なることで, 街の文化は少しだけ身近なものになります.',
    ],
    visualLabel: 'AFTER HOURS',
    visualTheme: 'violet',
  },
  {
    id: 'deep-sea-sound',
    source: 'Scope Science',
    sourceInitials: 'SS',
    publishedAt: '2時間前',
    readTime: 5,
    title: '深海の音から読み解く, 目に見えない生態系の変化',
    summary:
      '水中マイクが捉えた長期データから, 海の季節と生き物の移動が見えてきた.',
    body: [
      '光の届かない海では, 音が環境を知る大切な手がかりになります. 研究チームは長期間の録音から, 生き物の移動や人間活動の影響を分析しています.',
      '同じ場所の音を継続して比べることで, 一度の調査では見えない小さな変化を捉えられるようになりました.',
    ],
    visualLabel: 'BELOW 2,000M',
    visualTheme: 'aqua',
  },
  {
    id: 'repair-economy',
    source: 'Common Ledger',
    sourceInitials: 'CL',
    publishedAt: '3時間前',
    readTime: 7,
    title: '「直して使う」がつくる, 小さくて強い地域経済',
    summary:
      '修理する技術と場所を共有するリペアカフェ. モノを長く使うことから生まれる新しい循環.',
    body: [
      '壊れた家電や衣服を持ち寄り, 修理の知識を共有する場所が増えています. 費用を抑えるだけでなく, 技術や道具を地域で受け継ぐ役割もあります.',
      '大量に買い替える経済から, 手入れしながら使う経済へ. 小さな活動が地域の新しいつながりを育てています.',
    ],
    visualLabel: 'REPAIR / REUSE',
    visualTheme: 'amber',
  },
  {
    id: 'open-source-garden',
    source: 'Open Current',
    sourceInitials: 'OC',
    publishedAt: '5時間前',
    readTime: 4,
    title: 'オープンソースで育てる, 都市の小さな菜園',
    summary:
      'センサーと共有設計図を使って, 誰でも参加できる都市農園をつくる試み.',
    body: [
      '土の水分や日照を測る小さなセンサーを, 誰でも組み立てられる設計図として公開する活動が始まっています.',
      '技術は収穫量を競うためだけではありません. 初めて参加する人が植物の変化に気づき, 地域の経験を共有するきっかけにもなっています.',
    ],
    visualLabel: 'OPEN GARDEN',
    visualTheme: 'forest',
  },
]
