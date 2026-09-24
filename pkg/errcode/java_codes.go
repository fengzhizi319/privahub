// Code generated from secretpad-common errorcode enums and i18n/messages_{zh_CN,en_US}.properties. DO NOT EDIT BY HAND
// (regenerate from the Java sources when the Java error code table changes).

package errcode

// Java SecretPad error codes (secretpad-common/.../errorcode/*ErrorCode.java).
// Names are <Category><EnumName> in CamelCase; the message key is "<category>.<ENUM_NAME>".
var (
	AuthUserNotFound                           = define(202011600, "auth.USER_NOT_FOUND")
	AuthUserPasswordError                      = define(202011601, "auth.USER_PASSWORD_ERROR")
	AuthFailed                                 = define(202011602, "auth.AUTH_FAILED")
	AuthUserIsLocked                           = define(202011603, "auth.USER_IS_LOCKED")
	AuthResetPasswordIsLocked                  = define(202011604, "auth.RESET_PASSWORD_IS_LOCKED")
	ConcurrentTaskInterruptedError             = define(202012601, "concurrent.TASK_INTERRUPTED_ERROR")
	ConcurrentTaskExecutionError               = define(202012602, "concurrent.TASK_EXECUTION_ERROR")
	ConcurrentTaskTimeOutError                 = define(202012603, "concurrent.TASK_TIME_OUT_ERROR")
	DataFileNameEmpty                          = define(202011801, "data.FILE_NAME_EMPTY")
	DataFileTypeNotSupport                     = define(202011802, "data.FILE_TYPE_NOT_SUPPORT")
	DataFileExistsError                        = define(202011803, "data.FILE_EXISTS_ERROR")
	DataFileNotExistsError                     = define(202011804, "data.FILE_NOT_EXISTS_ERROR")
	DataIllegalParamsError                     = define(202011805, "data.ILLEGAL_PARAMS_ERROR")
	DataNameDuplicationError                   = define(202011806, "data.NAME_DUPLICATION_ERROR")
	DatasourceDataSourceEndpointConnectFail    = define(202012501, "datasource.DATA_SOURCE_ENDPOINT_CONNECT_FAIL")
	DatasourceDataSourceCreateFail             = define(202012502, "datasource.DATA_SOURCE_CREATE_FAIL")
	DatasourceDataSourceBucketNotExist         = define(202012503, "datasource.DATA_SOURCE_BUCKET_NOT_EXIST")
	DatasourceDataSourceCredentialsInvalid     = define(202012504, "datasource.DATA_SOURCE_CREDENTIALS_INVALID")
	DatasourceQueryDatasourceFailed            = define(202012505, "datasource.QUERY_DATASOURCE_FAILED")
	DatasourceDataSourceBucketNotMatchEndpoint = define(202012506, "datasource.DATA_SOURCE_BUCKET_NOT_MATCH_ENDPOINT")
	DatasourceDataSourceEndpointApiPortError   = define(202012507, "datasource.DATA_SOURCE_ENDPOINT_API_PORT_ERROR")
	DatasourceUnknownException                 = define(202012508, "datasource.DATASOURCE_UNKNOWN_EXCEPTION")
	DatasourceDataSourceDeleteFail             = define(202012509, "datasource.DATA_SOURCE_DELETE_FAIL")
	DatasourceDataSourceNotFound               = define(202012510, "datasource.DATA_SOURCE_NOT_FOUND")
	DatasourceUpdateDatasourceFailed           = define(202012511, "datasource.UPDATE_DATASOURCE_FAILED")
	DatatableNotExists                         = define(202011301, "datatable.DATATABLE_NOT_EXISTS")
	DatatableUnsupportedDatatableType          = define(202011302, "datatable.UNSUPPORTED_DATATABLE_TYPE")
	DatatableQueryDatatableFailed              = define(202011303, "datatable.QUERY_DATATABLE_FAILED")
	DatatableDeleteDatatableFailed             = define(202011304, "datatable.DELETE_DATATABLE_FAILED")
	DatatableDuplicatedAuthorized              = define(202011305, "datatable.DATATABLE_DUPLICATED_AUTHORIZED")
	DatatableQueryDatatableGrantFailed         = define(202011306, "datatable.QUERY_DATATABLE_GRANT_FAILED")
	DatatableCreateDatatableGrantFailed        = define(202011307, "datatable.CREATE_DATATABLE_GRANT_FAILED")
	DatatableCreateTeeJobFailed                = define(202011308, "datatable.CREATE_TEE_JOB_FAILED")
	DatatableCreateFailed                      = define(202011309, "datatable.DATATABLE_CREATE_FAILED")
	FeatureTableNotExist                       = define(202012301, "featuretable.FEATURE_TABLE_NOT_EXIST")
	FeatureTableIpFilter                       = define(202012302, "featuretable.FEATURE_TABLE_IP_FILTER")
	FeatureTableIpNotKnown                     = define(202012303, "featuretable.FEATURE_TABLE_IP_NOT_KNOWN")
	GraphComponentNotExists                    = define(202011701, "graph.COMPONENT_NOT_EXISTS")
	GraphNotExists                             = define(202011702, "graph.GRAPH_NOT_EXISTS")
	GraphExists                                = define(202011703, "graph.GRAPH_EXISTS")
	GraphNodeNotExists                         = define(202011704, "graph.GRAPH_NODE_NOT_EXISTS")
	GraphDatatableEmpty                        = define(202011705, "graph.GRAPH_DATATABLE_EMPTY")
	GraphComponent18nError                     = define(202011706, "graph.COMPONENT_18N_ERROR")
	GraphJobInvalid                            = define(202011707, "graph.GRAPH_JOB_INVALID")
	GraphNodeOutputNotExists                   = define(202011708, "graph.GRAPH_NODE_OUTPUT_NOT_EXISTS")
	GraphResultTypeNotSupported                = define(202011709, "graph.RESULT_TYPE_NOT_SUPPORTED")
	GraphDependentNodeNotRun                   = define(202011710, "graph.GRAPH_DEPENDENT_NODE_NOT_RUN")
	GraphNodeRouteNotExists                    = define(202011711, "graph.GRAPH_NODE_ROUTE_NOT_EXISTS")
	GraphNotOwnerCannotUpdate                  = define(202011712, "graph.GRAPH_NOT_OWNER_CANNOT_UPDATE")
	GraphNonOurCreationCanViewed               = define(202011713, "graph.NON_OUR_CREATION_CAN_VIEWED")
	InstNotExists                              = define(202011200, "inst.INST_NOT_EXISTS")
	InstNotMatchNode                           = define(202011201, "inst.INST_NOT_MATCH_NODE")
	InstNodeCountLimited                       = define(202011202, "inst.INST_NODE_COUNT_LIMITED")
	InstMismatchLogin                          = define(202011203, "inst.INST_MISMATCH_LOGIN")
	InstRegisterCheckFailed                    = define(202011204, "inst.INST_REGISTER_CHECK_FAILED")
	InstTokenMismatch                          = define(202011205, "inst.INST_TOKEN_MISMATCH")
	InstDuplicated                             = define(202011206, "inst.INST_DUPLICATED")
	InstFileOperationFailed                    = define(202011207, "inst.INST_FILE_OPERATION_FAILED")
	InstTokenUsed                              = define(202011208, "inst.INST_TOKEN_USED")
	InstInitiatorInstNodeMismatch              = define(202011209, "inst.INITIATOR_INST_NODE_MISMATCH")
	InstInviteeInstNodeMismatch                = define(202011210, "inst.INVITEE_INST_NODE_MISMATCH")
	KusciaRpcError                             = define(202012101, "kuscia.RPC_ERROR")
	ModelExportFailed                          = define(202012401, "model.MODEL_EXPORT_FAILED")
	NodeAlreadyExistError                      = define(202011401, "node.NODE_ALREADY_EXIST_ERROR")
	NodeCreateError                            = define(202011402, "node.NODE_CREATE_ERROR")
	NodeNotExistError                          = define(202011403, "node.NODE_NOT_EXIST_ERROR")
	NodeDeleteError                            = define(202011404, "node.NODE_DELETE_ERROR")
	NodeDomainDataNotExists                    = define(202011405, "node.DOMAIN_DATA_NOT_EXISTS")
	NodeUpdateError                            = define(202011406, "node.NODE_UPDATE_ERROR")
	NodeTokenError                             = define(202011407, "node.NODE_TOKEN_ERROR")
	NodeTokenIsEmptyError                      = define(202011408, "node.NODE_TOKEN_IS_EMPTY_ERROR")
	NodeCertConfigError                        = define(202011409, "node.NODE_CERT_CONFIG_ERROR")
	NodeRouteAlreadyExists                     = define(202012901, "nodeRoute.NODE_ROUTE_ALREADY_EXISTS")
	NodeRouteCreateError                       = define(202012902, "nodeRoute.NODE_ROUTE_CREATE_ERROR")
	NodeRouteNotExistError                     = define(202012903, "nodeRoute.NODE_ROUTE_NOT_EXIST_ERROR")
	NodeRouteDeleteError                       = define(202012904, "nodeRoute.NODE_ROUTE_DELETE_ERROR")
	NodeRouteUpdateError                       = define(202012905, "nodeRoute.NODE_ROUTE_UPDATE_ERROR")
	NodeRouteConfigError                       = define(202012906, "nodeRoute.NODE_ROUTE_CONFIG_ERROR")
	NodeRouteSrcNodeAndDestNodeSame            = define(202012907, "nodeRoute.SRC_NODE_AND_DEST_NODE_SAME")
	ProjectNotExists                           = define(202011501, "project.PROJECT_NOT_EXISTS")
	ProjectInstNotExists                       = define(202011502, "project.PROJECT_INST_NOT_EXISTS")
	ProjectNodeNotExists                       = define(202011503, "project.PROJECT_NODE_NOT_EXISTS")
	ProjectDatatableNotExists                  = define(202011504, "project.PROJECT_DATATABLE_NOT_EXISTS")
	ProjectResultNotFound                      = define(202011505, "project.PROJECT_RESULT_NOT_FOUND")
	ProjectGraphNotEmpty                       = define(202011506, "project.PROJECT_GRAPH_NOT_EMPTY")
	ProjectArchiveFail                         = define(202011507, "project.PROJECT_ARCHIVE_FAIL")
	ProjectUpdateFail                          = define(202011508, "project.PROJECT_UPDATE_FAIL")
	ProjectCanNotArchive                       = define(202011509, "project.PROJECT_CAN_NOT_ARCHIVE")
	ProjectCanNotCreateArchiveVote             = define(202011510, "project.PROJECT_CAN_NOT_CREATE_ARCHIVE_VOTE")
	ProjectModelNotFound                       = define(202011511, "project.PROJECT_MODEL_NOT_FOUND")
	ProjectServingNotFound                     = define(202011512, "project.PROJECT_SERVING_NOT_FOUND")
	ProjectServingNotSuccess                   = define(202011513, "project.PROJECT_SERVING_NOT_SUCCESS")
	ProjectServingNotOffline                   = define(202011514, "project.PROJECT_SERVING_NOT_OFFLINE")
	ProjectServingNotDiscard                   = define(202011515, "project.PROJECT_SERVING_NOT_DISCARD")
	ProjectFeatureTableNotExists               = define(202011516, "project.PROJECT_FEATURE_TABLE_NOT_EXISTS")
	ProjectNonOurCreationCanViewed             = define(202011517, "project.NON_OUR_CREATION_CAN_VIEWED")
	ProjectCreateFailed                        = define(202011518, "project.PROJECT_CREATE_FAILED")
	ProjectJobNotExists                        = define(202011901, "project_job.PROJECT_JOB_NOT_EXISTS")
	ProjectJobCreateError                      = define(202011902, "project_job.PROJECT_JOB_CREATE_ERROR")
	ProjectJobTaskNotExists                    = define(202011903, "project_job.PROJECT_JOB_TASK_NOT_EXISTS")
	ProjectJobDeleteError                      = define(202011904, "project_job.PROJECT_JOB_DELETE_ERROR")
	ProjectJobCloudLogError                    = define(202011905, "project_job.PROJECT_JOB_CLOUD_LOG_ERROR")
	ProjectJobNodePermissionError              = define(202011906, "project_job.PROJECT_JOB_NODE_PERMISSION_ERROR")
	ResultScqlResultEmpty                      = define(202013001, "result.SCQL_RESULT_EMPTY")
	ResultScqlResultError                      = define(202013002, "result.SCQL_RESULT_ERROR")
	ScheduledProjectJobNeedSuccessOnce         = define(202015001, "scheduled.PROJECT_JOB_NEED_SUCCESS_ONCE")
	ScheduledProjectJobNotExist                = define(202015002, "scheduled.PROJECT_JOB_NOT_EXIST")
	ScheduledScheduleNotExist                  = define(202015003, "scheduled.SCHEDULE_NOT_EXIST")
	ScheduledScheduleUpNotDel                  = define(202015004, "scheduled.SCHEDULE_UP_NOT_DEL")
	ScheduledScheduleRunningNotOffline         = define(202015005, "scheduled.SCHEDULE_RUNNING_NOT_OFFLINE")
	ScheduledRequestIsNull                     = define(202015006, "scheduled.REQUEST_IS_NULL")
	ScheduledScheduleTaskNotExist              = define(202015007, "scheduled.SCHEDULE_TASK_NOT_EXIST")
	ScheduledScheduleTaskStatusNotRunning      = define(202015008, "scheduled.SCHEDULE_TASK_STATUS_NOT_RUNNING")
	ScheduledProjectJobRestartError            = define(202015009, "scheduled.PROJECT_JOB_RESTART_ERROR")
	ScheduledUserNotOwner                      = define(202015010, "scheduled.USER_NOT_OWNER")
	ScheduledScheduleCreateError               = define(202015011, "scheduled.SCHEDULE_CREATE_ERROR")
	SystemValidationError                      = define(202011100, "system.VALIDATION_ERROR")
	SystemUnknownError                         = define(202011101, "system.UNKNOWN_ERROR")
	SystemOutOfRangeError                      = define(202011102, "system.OUT_OF_RANGE_ERROR")
	SystemHttp4xxError                         = define(202011103, "system.HTTP_4XX_ERROR")
	SystemHttp404Error                         = define(202011104, "system.HTTP_404_ERROR")
	SystemHttp5xxError                         = define(202011105, "system.HTTP_5XX_ERROR")
	SystemEncodeError                          = define(202011106, "system.ENCODE_ERROR")
	SystemSseError                             = define(202011109, "system.SSE_ERROR")
	SystemSyncError                            = define(202011110, "system.SYNC_ERROR")
	SystemTodoError                            = define(202011199, "system.TODO_ERROR")
	SystemRemoteCallError                      = define(2020111011, "system.REMOTE_CALL_ERROR")
	SystemRequestFrequencyError                = define(2020111012, "system.REQUEST_FREQUENCY_ERROR")
	SystemSignatureError                       = define(2020111107, "system.SIGNATURE_ERROR")
	SystemVerifySignatureError                 = define(2020111108, "system.VERIFY_SIGNATURE_ERROR")
	UserUpdatePasswordErrorInconsistent        = define(202012001, "user.USER_UPDATE_PASSWORD_ERROR_INCONSISTENT")
	UserUpdatePasswordErrorSame                = define(202012002, "user.USER_UPDATE_PASSWORD_ERROR_SAME")
	UserUpdatePasswordErrorIncorrect           = define(202012003, "user.USER_UPDATE_PASSWORD_ERROR_INCORRECT")
	VoteNotExists                              = define(202012201, "vote.VOTE_NOT_EXISTS")
	VoteCheckFailed                            = define(202012202, "vote.VOTE_CHECK_FAILED")
	VoteSignatureSynchronizing                 = define(202012203, "vote.VOTE_SIGNATURE_SYNCHRONIZING")
	VoteProjectVoteNotExists                   = define(202012204, "vote.PROJECT_VOTE_NOT_EXISTS")
	VoteProjectArchiveVoteAlreadyExist         = define(202012205, "vote.PROJECT_ARCHIVE_VOTE_ALREADY_EXIST")
	VoteParticipantNotExist                    = define(202012206, "vote.PARTICIPANT_NOT_EXIST")
	VoteInitiatorNodeDuplicate                 = define(202012207, "vote.INITIATOR_NODE_DUPLICATE")
	VoteInitiatorInstNotFound                  = define(202012208, "vote.VOTE_INITIATOR_INST_NOT_FOUND")
	VoteMasterRouteNotReady                    = define(202012209, "vote.VOTE_MASTER_ROUTE_NOT_READY")
)

// messages maps message key -> {zh_CN, en_US} templates (java.text.MessageFormat style {0} placeholders).
var messages = map[string][2]string{
	"auth.USER_NOT_FOUND":                              {"用户名或密码错误，剩余重试次数{0}次", "Username or password error,remaining retry {0} attempts"},
	"auth.USER_PASSWORD_ERROR":                         {"用户名或密码错误，剩余重试次数{0}次", "Username or password error,remaining retry {0} attempts"},
	"auth.AUTH_FAILED":                                 {"用户认证失败: {0}", "Authentication user failed: {0}"},
	"auth.USER_IS_LOCKED":                              {"登录已被锁定，请{0}分钟后再试", "Login is locked, Please try again in {0} minutes"},
	"auth.RESET_PASSWORD_IS_LOCKED":                    {"重置密码功能被锁定,请{0}分钟后再试", "The reset password function has been locked, please try again in {0} minutes"},
	"concurrent.TASK_INTERRUPTED_ERROR":                {"任务被异常打断", "Task interrupted"},
	"concurrent.TASK_EXECUTION_ERROR":                  {"任务执行发生异常", "Task execution error"},
	"concurrent.TASK_TIME_OUT_ERROR":                   {"任务执行超时", "Task time out"},
	"data.FILE_NAME_EMPTY":                             {"文件名为空", "File name is empty"},
	"data.FILE_TYPE_NOT_SUPPORT":                       {"文件类型不支持: {0}", "File type is not support: {0}"},
	"data.FILE_EXISTS_ERROR":                           {"文件已存在", "File already exists"},
	"data.FILE_NOT_EXISTS_ERROR":                       {"文件不存在", "File not exists"},
	"data.ILLEGAL_PARAMS_ERROR":                        {"参数不合法: {0}", "Illegal parameters: {0}"},
	"data.NAME_DUPLICATION_ERROR":                      {"数据名称已存在", "Data name duplication."},
	"datasource.DATA_SOURCE_ENDPOINT_CONNECT_FAIL":     {"连接endpoint失败,请检查endpoint地址,或者OSS服务是否正常", "connect to endpoint fail,please check your endpoint and oss service"},
	"datasource.DATA_SOURCE_CREATE_FAIL":               {"数据源创建失败: {0}", "datasource create fail : {0}"},
	"datasource.DATA_SOURCE_BUCKET_NOT_EXIST":          {"oss bucket 不存在", "oss bucket not exists"},
	"datasource.DATA_SOURCE_CREDENTIALS_INVALID":       {"数据源凭证无效", "datasource credentials invalid"},
	"datasource.QUERY_DATASOURCE_FAILED":               {"查询数据源失败", "Query datasource failed"},
	"datasource.DATA_SOURCE_BUCKET_NOT_MATCH_ENDPOINT": {"bucket和endpoint不匹配", "bucket not match endpoint"},
	"datasource.DATA_SOURCE_ENDPOINT_API_PORT_ERROR":   {"S3 API请求与endpoint端口不匹配.", "S3 API Requests must be made to API port."},
	"datasource.DATASOURCE_UNKNOWN_EXCEPTION":          {"S3 API 异常 : {0}", "S3 API error : {0}"},
	"datasource.DATA_SOURCE_DELETE_FAIL":               {"数据源删除失败: {0}", "datasource delete fail: {0}"},
	"datasource.DATA_SOURCE_NOT_FOUND":                 {"数据源不存在", "Data source not found"},
	"datasource.UPDATE_DATASOURCE_FAILED":              {"更新数据源失败: {0}", "Data source update failed: {0}"},
	"datatable.DATATABLE_NOT_EXISTS":                   {"数据表不存在", "Datatable not exist"},
	"datatable.UNSUPPORTED_DATATABLE_TYPE":             {"不支持的数据表类型: {0}", "Unsupported datatable type: {0}"},
	"datatable.QUERY_DATATABLE_FAILED":                 {"查询数据表失败", "Query datatable failed"},
	"datatable.DELETE_DATATABLE_FAILED":                {"数据表删除失败", "Delete datatable failed"},
	"datatable.DATATABLE_DUPLICATED_AUTHORIZED":        {"数据表已被授权到项目", "Datatable has been authorized to project"},
	"datatable.QUERY_DATATABLE_GRANT_FAILED":           {"查询数据表授权失败", "Query datatable grant failed"},
	"datatable.CREATE_DATATABLE_GRANT_FAILED":          {"创建数据表授权失败", "Create datatable grant failed"},
	"datatable.CREATE_TEE_JOB_FAILED":                  {"创建tee任务失败", "Create tee job failed"},
	"datatable.DATATABLE_CREATE_FAILED":                {"数据表创建失败", "Create datatable failed"},
	"featuretable.FEATURE_TABLE_NOT_EXIST":             {"在线特征表不存在", "feature table not exist"},
	"featuretable.FEATURE_TABLE_IP_FILTER":             {"禁止内网IP", "feature table http url ip is not allow"},
	"featuretable.FEATURE_TABLE_IP_NOT_KNOWN":          {"无效URL", "feature table http url ip is not effective"},
	"graph.COMPONENT_NOT_EXISTS":                       {"组件不存在: {0}", "Component not found: {0}"},
	"graph.GRAPH_NOT_EXISTS":                           {"训练图不存在", "Graph not exists"},
	"graph.GRAPH_EXISTS":                               {"训练图已经存在", "Graph already exists"},
	"graph.GRAPH_NODE_NOT_EXISTS":                      {"训练图节点不可用: {0}", "Graph node not ready:{0}"},
	"graph.GRAPH_DATATABLE_EMPTY":                      {"读样本组件未配置", "Graph datatable can not be empty"},
	"graph.COMPONENT_18N_ERROR":                        {"组件i18n配置错误: {0}", "Graph component 18n config error: {0}"},
	"graph.GRAPH_JOB_INVALID":                          {"训练图节点不合法: {0}", "Graph job invalid: {0}"},
	"graph.GRAPH_NODE_OUTPUT_NOT_EXISTS":               {"图节点产出物不存在", "Graph node output not exists"},
	"graph.RESULT_TYPE_NOT_SUPPORTED":                  {"训练图产出物类型不支持", "Graph result type not supported"},
	"graph.GRAPH_DEPENDENT_NODE_NOT_RUN":               {"依赖组件未执行: {0}", "Graph dependent node not run: {0}"},
	"graph.GRAPH_NODE_ROUTE_NOT_EXISTS":                {"训练图节点路由不可用: {0}", "Graph node route not ready:{0}"},
	"graph.GRAPH_NOT_OWNER_CANNOT_UPDATE":              {"非创建方不能修改: {0}", "Cannot modify by which is not the creator: {0}"},
	"graph.NON_OUR_CREATION_CAN_VIEWED":                {"非我方节点创建 仅可查看", "Non our node creation can only be viewed"},
	"inst.INST_NOT_EXISTS":                             {"机构不存在", "Inst not exist"},
	"inst.INST_NOT_MATCH_NODE":                         {"机构和某个节点不匹配", "inst not match node"},
	"inst.INST_NODE_COUNT_LIMITED":                     {"机构下节点超过数量", "inst node exceed"},
	"inst.INST_MISMATCH_LOGIN":                         {"i传入机构与登录机构不匹配:{0}", "instId mismatch login instId:{0}"},
	"inst.INST_REGISTER_CHECK_FAILED":                  {"机构节点注册参数检查失败", "inst node register check failed"},
	"inst.INST_TOKEN_MISMATCH":                         {"机构注册码不匹配", "inst token mismatch"},
	"inst.INST_DUPLICATED":                             {"机构重复", "inst duplicated"},
	"inst.INST_FILE_OPERATION_FAILED":                  {"机构证书文件操作失败", "inst cert file operation failed"},
	"inst.INST_TOKEN_USED":                             {"机构注册码已被使用", "token used"},
	"inst.INITIATOR_INST_NODE_MISMATCH":                {"发起方机构和节点不匹配", "The initiator organization and node do not match"},
	"inst.INVITEE_INST_NODE_MISMATCH":                  {"受邀方机构和节点不匹配", "The invited organization and node do not match"},
	"kuscia.RPC_ERROR":                                 {"远程调用Kuscia错误: {0}", "Kuscia grpc error: {0}"},
	"model.MODEL_EXPORT_FAILED":                        {"模型打包失败:{0}", "Model export error:{0}"},
	"node.NODE_ALREADY_EXIST_ERROR":                    {"节点已存在: {0}", "Node already exist: {0}"},
	"node.NODE_CREATE_ERROR":                           {"节点创建失败: {0}", "Node create error: {0}"},
	"node.NODE_NOT_EXIST_ERROR":                        {"节点不存在: {0}", "Node not exist: {0}"},
	"node.NODE_DELETE_ERROR":                           {"节点删除失败: {0}", "Node delete error: {0}"},
	"node.DOMAIN_DATA_NOT_EXISTS":                      {"Domain data不存在 {0}", "Domain data not exits: {0}"},
	"node.NODE_UPDATE_ERROR":                           {"节点更新失败: {0}", "Node update error: {0}"},
	"node.NODE_TOKEN_ERROR":                            {"获取token失败: {0}", "Kuscia return token error: {0}"},
	"node.NODE_TOKEN_IS_EMPTY_ERROR":                   {"Kuscia返回token为空 {0}", "Kuscia return empty token: {0}"},
	"node.NODE_CERT_CONFIG_ERROR":                      {"请上传正确的公钥证书", "Node cert config error"},
	"nodeRoute.NODE_ROUTE_ALREADY_EXISTS":              {"路由已经存在 {0}", "NodeRoute already exist: {0}"},
	"nodeRoute.NODE_ROUTE_CREATE_ERROR":                {"路由创建失败 {0}", "NodeRoute create error: {0}"},
	"nodeRoute.NODE_ROUTE_NOT_EXIST_ERROR":             {"路由不存在 {0}", "NodeRoute not exist: {0}"},
	"nodeRoute.NODE_ROUTE_DELETE_ERROR":                {"路由删除失败 {0}", "NodeRoute delete error: {0}"},
	"nodeRoute.NODE_ROUTE_UPDATE_ERROR":                {"路由更新失败 {0}", "NodeRoute update error: {0}"},
	"nodeRoute.NODE_ROUTE_CONFIG_ERROR":                {"节点通信地址配置错误", "NodeRoute config error"},
	"nodeRoute.SRC_NODE_AND_DEST_NODE_SAME":            {"源节点和目标节点不能相同", "Source node and destination node cannot be the same"},
	"project.PROJECT_NOT_EXISTS":                       {"项目不存在", "Project not exist"},
	"project.PROJECT_INST_NOT_EXISTS":                  {"项目机构不存在", "Project inst not exist"},
	"project.PROJECT_NODE_NOT_EXISTS":                  {"节点没有参与该项目", "Node not join Project"},
	"project.PROJECT_DATATABLE_NOT_EXISTS":             {"项目数据表不存在", "Project datatable not exist"},
	"project.PROJECT_RESULT_NOT_FOUND":                 {"项目结果未找到: {0}", "Project result not found: {0}"},
	"project.PROJECT_GRAPH_NOT_EMPTY":                  {"项目存在训练流,无法删除", "Project has graph, can't delete"},
	"project.PROJECT_ARCHIVE_FAIL":                     {"只有项目的所有者才能直接归档项目", "Only project owner can archive the project directly"},
	"project.PROJECT_UPDATE_FAIL":                      {"只有项目的所有者才能编辑项目", "Only project owner can update the project"},
	"project.PROJECT_CAN_NOT_ARCHIVE":                  {"之后审批中的项目可以被直接归档", "Only reviewing project can be archived directly"},
	"project.PROJECT_CAN_NOT_CREATE_ARCHIVE_VOTE":      {"只有当前状态是达成合作的项目才能发起归档审批", "Only approved project can Initiate approval"},
	"project.PROJECT_MODEL_NOT_FOUND":                  {"项目中不存在该模型", "project model not exist"},
	"project.PROJECT_SERVING_NOT_FOUND":                {"不存在的服务", "project serving not exist"},
	"project.PROJECT_SERVING_NOT_SUCCESS":              {"模型服务不是成功状态,无法删除", "project serving status not success"},
	"project.PROJECT_SERVING_NOT_OFFLINE":              {"模型服务不是下线状态,无法废弃", "project serving status not offline"},
	"project.PROJECT_SERVING_NOT_DISCARD":              {"模型服务不是废弃状态,无法删除", "project serving status not discard"},
	"project.PROJECT_FEATURE_TABLE_NOT_EXISTS":         {"项目特征表不存在", "project feature table not exist"},
	"project.NON_OUR_CREATION_CAN_VIEWED":              {"非我方节点创建 仅可查看", "Non our node creation can only be viewed"},
	"project.PROJECT_CREATE_FAILED":                    {"项目创建失败:", "project create failed"},
	"project_job.PROJECT_JOB_NOT_EXISTS":               {"项目任务不存在", "Project job not exist"},
	"project_job.PROJECT_JOB_CREATE_ERROR":             {"项目任务创建失败: {0}", "Project job create error: {0}"},
	"project_job.PROJECT_JOB_TASK_NOT_EXISTS":          {"项目子任务不存在", "Project job task not exist"},
	"project_job.PROJECT_JOB_DELETE_ERROR":             {"项目任务删除失败: {0}", "Project job delete error: {0}"},
	"project_job.PROJECT_JOB_CLOUD_LOG_ERROR":          {"获取云端日志失败: {0}", "Project job get cloud log fail: {0}"},
	"project_job.PROJECT_JOB_NODE_PERMISSION_ERROR":    {"无获取日志权限", "no permission to fetch log"},
	"result.SCQL_RESULT_EMPTY":                         {"SCQL结果为空", "SCQL result is empty"},
	"result.SCQL_RESULT_ERROR":                         {"SCQL结果出错: {0}", "SCQL result error: {0}"},
	"scheduled.PROJECT_JOB_NEED_SUCCESS_ONCE":          {"画布至少需要成功运行一次", "project job need success once"},
	"scheduled.PROJECT_JOB_NOT_EXIST":                  {"当前无法查询到画布Job", "project job not exist"},
	"scheduled.SCHEDULE_NOT_EXIST":                     {"不存在的定时任务", "schedule not exist"},
	"scheduled.SCHEDULE_UP_NOT_DEL":                    {"有任务在运行无法删除", "schedule is running can not remove"},
	"scheduled.SCHEDULE_RUNNING_NOT_OFFLINE":           {"有任务在运行无法删除", "schedule is running can not offline"},
	"scheduled.REQUEST_IS_NULL":                        {"构建kuscia请求异常是null", "schedule request is null"},
	"scheduled.SCHEDULE_TASK_NOT_EXIST":                {"定时任务不存在", "schedule task not exist"},
	"scheduled.SCHEDULE_TASK_STATUS_NOT_RUNNING":       {"定时任务不是运行状态，当前状态是: {0}", "schedule task status not running: {0}"},
	"scheduled.PROJECT_JOB_RESTART_ERROR":              {"定时任务重跑失败: {0}", "schedule job restart error: {0}"},
	"scheduled.USER_NOT_OWNER":                         {"只有发起方可以操作", "only schedule owner can operate"},
	"scheduled.SCHEDULE_CREATE_ERROR":                  {"创建定时任务失败: {0}", "schedule create error: {0}"},
	"system.VALIDATION_ERROR":                          {"入参校验失败: {0}", "Input parameter verification failed: {0}"},
	"system.UNKNOWN_ERROR":                             {"系统未知错误: {0}", "System unknown error: {0}"},
	"system.OUT_OF_RANGE_ERROR":                        {"超出范围: {0}", "Out of range error: {0}"},
	"system.HTTP_4XX_ERROR":                            {"错误请求: {0}", "Bad request: {0}"},
	"system.HTTP_404_ERROR":                            {"404: {0}", "404 not found: {0}"},
	"system.HTTP_5XX_ERROR":                            {"系统错误: {0}", "System error: {0}"},
	"system.ENCODE_ERROR":                              {"编码错误: {0}", "Encode error: {0}"},
	"system.SSE_ERROR":                                 {"SSE数据同步错误: {0}", "SSE sync error: {0}"},
	"system.SYNC_ERROR":                                {"数据同步错误:{0}", "sync error:{0}"},
	"system.TODO_ERROR":                                {"待办事宜错误: {0}", "Todo error: {0}"},
	"system.REMOTE_CALL_ERROR":                         {"远程调用错误:{0}", "Remote call error:{0}"},
	"system.REQUEST_FREQUENCY_ERROR":                   {"请求太频繁,请稍后再试", "request too frequent, please try again late"},
	"system.SIGNATURE_ERROR":                           {"签名错误: {0}", "Signature error: {0}"},
	"system.VERIFY_SIGNATURE_ERROR":                    {"签名校验错误: {0}", "Due to system upgrade or message tampering, project data is lost, recommended to initiate a new project invitation"},
	"user.USER_UPDATE_PASSWORD_ERROR_INCONSISTENT":     {"两次输入的密码不一致", "The passwords entered twice are inconsistent"},
	"user.USER_UPDATE_PASSWORD_ERROR_SAME":             {"新密码和旧密码不能相同", "The new and old passwords cannot be the same"},
	"user.USER_UPDATE_PASSWORD_ERROR_INCORRECT":        {"输入的旧密码不正确", "The incorrect old password input"},
	"vote.VOTE_NOT_EXISTS":                             {"不存在的投票", "Vote not exists"},
	"vote.VOTE_CHECK_FAILED":                           {"投票校验失败", "Vote check fail"},
	"vote.VOTE_SIGNATURE_SYNCHRONIZING":                {"投票签名信息同步中", "Vote signature in synchronizing"},
	"vote.PROJECT_VOTE_NOT_EXISTS":                     {"项目不在审批列表信息中:{0}", "Project vote not exists:{0}"},
	"vote.PROJECT_ARCHIVE_VOTE_ALREADY_EXIST":          {"该项目归档审批已经存在:{0}", "The project archive vote already exist:{0}"},
	"vote.PARTICIPANT_NOT_EXIST":                       {"投票参与方不存在", "Voting participant does not exist"},
	"vote.INITIATOR_NODE_DUPLICATE":                    {"发起方节点重复", "Duplicate initiator node"},
	"vote.VOTE_INITIATOR_INST_NOT_FOUND":               {"未知的发起方机构: {0}", "Initiator node not found:{0}"},
	"vote.VOTE_MASTER_ROUTE_NOT_READY":                 {"主节点路由不可用: {0}", "Master route not ready:{0}"},
}
