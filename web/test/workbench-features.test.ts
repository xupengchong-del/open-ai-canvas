import { expect, test } from "bun:test";

import { defaultFeatureAvailability } from "../src/stores/use-user-store";

// 旧口径「新工作台入口默认关闭」已被「默认开放电商」取代。
// 后端 requireTaskGenerationFeatures 对电商商品图/视频也校验 ImageWorkbench /
// VideoWorkbench 与 EcommerceVideo，四项必须一起打开，电商 Agent 才完整可用。
test("电商 Agent 及其依赖的工作台默认开放", () => {
    expect(defaultFeatureAvailability.ecommerceEnabled).toBe(true);
    expect(defaultFeatureAvailability.ecommerceVideoEnabled).toBe(true);
    expect(defaultFeatureAvailability.imageWorkbenchEnabled).toBe(true);
    expect(defaultFeatureAvailability.videoWorkbenchEnabled).toBe(true);
});
