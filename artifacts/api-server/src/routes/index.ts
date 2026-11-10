import { Router, type IRouter } from "express";
import fraudGuardDemoRouter from "./fraudguard-demo";
import healthRouter from "./health";

const router: IRouter = Router();

router.use(healthRouter);
router.use(fraudGuardDemoRouter);

export default router;
