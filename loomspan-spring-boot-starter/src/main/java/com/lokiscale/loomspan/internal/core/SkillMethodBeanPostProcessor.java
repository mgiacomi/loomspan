package com.lokiscale.loomspan.internal.core;

import tools.jackson.core.JacksonException;
import tools.jackson.databind.JsonNode;
import tools.jackson.databind.ObjectMapper;
import tools.jackson.databind.JavaType;
import tools.jackson.databind.node.ArrayNode;
import tools.jackson.databind.node.ObjectNode;
import tools.jackson.databind.introspect.BeanPropertyDefinition;
import com.lokiscale.loomspan.api.SkillMethod;
import com.lokiscale.loomspan.api.SkillParam;
import com.lokiscale.loomspan.internal.runtime.input.SkillInputContractResolver;
import com.lokiscale.loomspan.internal.serialization.LoomspanMethodInputSchemaGenerator;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.BeansException;
import org.springframework.beans.factory.BeanFactory;
import org.springframework.beans.factory.BeanFactoryAware;
import org.springframework.beans.factory.config.BeanPostProcessor;
import org.springframework.aop.support.AopUtils;
import org.springframework.core.BridgeMethodResolver;
import org.springframework.core.MethodIntrospector;
import org.springframework.core.annotation.AnnotatedElementUtils;
import org.springframework.core.io.Resource;
import org.springframework.util.ReflectionUtils;
import org.springframework.util.StreamUtils;
import org.springframework.util.ClassUtils;

import java.io.IOException;
import java.io.InputStream;
import java.lang.reflect.Method;
import java.lang.reflect.Parameter;
import java.nio.charset.StandardCharsets;
import java.util.ArrayList;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Objects;
import java.util.Set;
import java.util.concurrent.ConcurrentHashMap;
import java.util.stream.Collectors;

public class SkillMethodBeanPostProcessor implements BeanPostProcessor, BeanFactoryAware
{
    private static final Logger log = LoggerFactory.getLogger(SkillMethodBeanPostProcessor.class);

    private final SkillImplementationTargetRegistry targetRegistry;
    private final ObjectMapper objectMapper;
    private final LoomspanExceptionTransformer LoomspanExceptionTransformer;
    private final SkillInputContractResolver inputContractResolver;
    private final LoomspanMethodInputSchemaGenerator schemaGenerator;
    private final Set<String> processedBeanNames = ConcurrentHashMap.newKeySet();
    private BeanFactory beanFactory;

    public SkillMethodBeanPostProcessor(SkillImplementationTargetRegistry targetRegistry)
    {
        this(targetRegistry,
                com.lokiscale.loomspan.internal.serialization.LoomspanJacksonCodecs.defaults().applicationConversion(),
                new DefaultLoomspanExceptionTransformer(), new SkillInputContractResolver());
    }

    public static SkillMethodBeanPostProcessor create(SkillImplementationTargetRegistry targetRegistry,
            LoomspanExceptionTransformer LoomspanExceptionTransformer)
    {
        return new SkillMethodBeanPostProcessor(
                targetRegistry,
                com.lokiscale.loomspan.internal.serialization.LoomspanJacksonCodecs.defaults().applicationConversion(),
                LoomspanExceptionTransformer,
                new SkillInputContractResolver());
    }

    public static SkillMethodBeanPostProcessor create(SkillImplementationTargetRegistry targetRegistry,
            ObjectMapper objectMapper,
            LoomspanExceptionTransformer LoomspanExceptionTransformer,
            SkillInputContractResolver inputContractResolver)
    {
        return new SkillMethodBeanPostProcessor(
                targetRegistry,
                objectMapper,
                LoomspanExceptionTransformer,
                inputContractResolver);
    }

    public static SkillMethodBeanPostProcessor create(SkillImplementationTargetRegistry targetRegistry,
            ObjectMapper applicationMapper,
            ObjectMapper schemaMapper,
            LoomspanExceptionTransformer LoomspanExceptionTransformer,
            SkillInputContractResolver inputContractResolver)
    {
        return new SkillMethodBeanPostProcessor(targetRegistry, applicationMapper, schemaMapper,
                LoomspanExceptionTransformer, inputContractResolver);
    }

    SkillMethodBeanPostProcessor(SkillImplementationTargetRegistry targetRegistry,
            ObjectMapper objectMapper,
            LoomspanExceptionTransformer LoomspanExceptionTransformer,
            SkillInputContractResolver inputContractResolver)
    {
        this(targetRegistry, objectMapper, objectMapper, LoomspanExceptionTransformer, inputContractResolver);
    }

    private SkillMethodBeanPostProcessor(SkillImplementationTargetRegistry targetRegistry,
            ObjectMapper objectMapper,
            ObjectMapper schemaMapper,
            LoomspanExceptionTransformer LoomspanExceptionTransformer,
            SkillInputContractResolver inputContractResolver)
    {
        this.targetRegistry = Objects.requireNonNull(targetRegistry, "targetRegistry must not be null");
        this.objectMapper = Objects.requireNonNull(objectMapper, "objectMapper must not be null");
        this.LoomspanExceptionTransformer = Objects.requireNonNull(LoomspanExceptionTransformer, "LoomspanExceptionTransformer must not be null");
        this.inputContractResolver = Objects.requireNonNull(inputContractResolver, "inputContractResolver must not be null");
        this.schemaGenerator = new LoomspanMethodInputSchemaGenerator(
                Objects.requireNonNull(schemaMapper, "schemaMapper must not be null"));
    }

    @Override
    public void setBeanFactory(BeanFactory beanFactory) throws BeansException
    {
        this.beanFactory = Objects.requireNonNull(beanFactory, "beanFactory must not be null");
    }

    @Override
    public Object postProcessAfterInitialization(Object bean, String beanName) throws BeansException
    {
        synchronized (processedBeanNames)
        {
            if (processedBeanNames.contains(beanName))
            {
                return bean;
            }

            discoverAndRegisterTargets(bean, beanName);
            processedBeanNames.add(beanName);
        }
        return bean;
    }

    private void discoverAndRegisterTargets(Object bean, String beanName)
    {
        Class<?> targetClass = AopUtils.getTargetClass(bean);
        Map<Method, SkillMethod> discovered = MethodIntrospector.selectMethods(
                targetClass,
                (MethodIntrospector.MetadataLookup<SkillMethod>) method ->
                        AnnotatedElementUtils.findMergedAnnotation(method, SkillMethod.class));

        Map<Method, SkillMethod> canonicalMethods = new LinkedHashMap<>();
        discovered.forEach((method, annotation) -> {
            Method canonical = BridgeMethodResolver.findBridgedMethod(method);
            if (!canonical.isBridge() && !canonical.isSynthetic())
            {
                canonicalMethods.putIfAbsent(canonical, annotation);
            }
        });

        canonicalMethods.keySet().stream()
                .collect(Collectors.groupingBy(Method::getName, LinkedHashMap::new, Collectors.toList()))
                .forEach((methodName, methods) -> {
                    if (methods.size() > 1)
                    {
                        String targetId = beanName + "#" + methodName;
                        throw new IllegalStateException("Invalid @SkillMethod targets on bean '" + beanName
                                + "': methods named '" + methodName + "' produce the ambiguous target ID '"
                                + targetId + "'. Annotated target method names must be unique within a Spring bean; rename one method.");
                    }
                });

        canonicalMethods.forEach((method, annotation) ->
                registerSkillMethod(beanName, method, resolveContractMethod(beanName, targetClass, method), annotation));
    }

    private void registerSkillMethod(String beanName, Method method, Method contractMethod, SkillMethod annotation)
    {
        validateSkillParameters(beanName, method, contractMethod);
        String capabilityDescription = annotation.description().isBlank() ? method.getName() : annotation.description();
        String inputSchema = buildInputSchema(method, contractMethod);
        String targetId = beanName + "#" + method.getName();
        CapabilityInvoker capabilityInvoker = arguments ->
                invokeSkillMethod(beanName, method, contractMethod, targetId, arguments);

        SkillImplementationTarget target = new SkillImplementationTarget(
                targetId,
                capabilityDescription,
                capabilityInvoker,
                inputSchema,
                inputContractResolver.resolveJavaCapability(inputSchema));

        targetRegistry.register(target);
    }

    private void validateSkillParameters(String beanName, Method method, Method contractMethod)
    {
        Parameter[] parameters = method.getParameters();
        Parameter[] contractParameters = contractMethod.getParameters();
        for (int index = 0; index < parameters.length; index++)
        {
            SkillParam skillParam = contractParameters[index].getAnnotation(SkillParam.class);
            if (skillParam == null)
            {
                skillParam = parameters[index].getAnnotation(SkillParam.class);
            }
            if (skillParam != null && !skillParam.required() && parameters[index].getType().isPrimitive())
            {
                throw new IllegalStateException("Invalid @SkillParam contract on bean '" + beanName
                        + "' for method '" + method.getName() + "', parameter '"
                        + contractParameters[index].getName() + "': required=false cannot be used with primitive "
                        + parameters[index].getType().getTypeName()
                        + " because an omitted value binds to null; use the boxed type or mark the parameter required.");
            }
        }
    }

    private Method resolveContractMethod(String beanName, Class<?> targetClass, Method canonicalMethod)
    {
        List<Method> interfaceContracts = ClassUtils.getAllInterfacesForClassAsSet(targetClass).stream()
                .flatMap(type -> java.util.Arrays.stream(type.getMethods()))
                .filter(candidate -> AnnotatedElementUtils.findMergedAnnotation(candidate, SkillMethod.class) != null)
                .filter(candidate -> mapsToCanonicalMethod(candidate, targetClass, canonicalMethod))
                .distinct()
                .sorted(java.util.Comparator.comparing(Method::toGenericString))
                .toList();

        if (canonicalMethod.isAnnotationPresent(SkillMethod.class)
                && interfaceContracts.stream().noneMatch(this::hasSkillParameterMetadata))
        {
            return canonicalMethod;
        }

        if (interfaceContracts.size() > 1)
        {
            Method firstContract = interfaceContracts.getFirst();
            boolean compatible = interfaceContracts.stream()
                    .skip(1)
                    .allMatch(candidate -> contractsEquivalent(firstContract, candidate));
            if (!compatible)
            {
                throw new IllegalStateException("Invalid @SkillMethod contract on bean '" + beanName
                        + "' for method '" + canonicalMethod.getName()
                        + "': annotated interfaces declare incompatible method or parameter metadata. "
                        + "Consolidate the declarations into one public interface contract.");
            }
        }

        return interfaceContracts.stream()
                .filter(this::hasSkillParameterMetadata)
                .findFirst()
                .orElseGet(() -> interfaceContracts.stream().findFirst().orElse(canonicalMethod));
    }

    private boolean contractsEquivalent(Method left, Method right)
    {
        if (!Objects.equals(
                AnnotatedElementUtils.findMergedAnnotation(left, SkillMethod.class),
                AnnotatedElementUtils.findMergedAnnotation(right, SkillMethod.class)))
        {
            return false;
        }

        Parameter[] leftParameters = left.getParameters();
        Parameter[] rightParameters = right.getParameters();
        if (leftParameters.length != rightParameters.length)
        {
            return false;
        }

        for (int index = 0; index < leftParameters.length; index++)
        {
            if (!leftParameters[index].getName().equals(rightParameters[index].getName())
                    || !Objects.equals(
                            leftParameters[index].getAnnotation(SkillParam.class),
                            rightParameters[index].getAnnotation(SkillParam.class)))
            {
                return false;
            }
        }
        return true;
    }

    private boolean hasSkillParameterMetadata(Method method)
    {
        return java.util.Arrays.stream(method.getParameters())
                .anyMatch(parameter -> parameter.isAnnotationPresent(SkillParam.class));
    }

    private boolean mapsToCanonicalMethod(Method interfaceMethod, Class<?> targetClass, Method canonicalMethod)
    {
        Method implementationMethod = ReflectionUtils.findMethod(
                targetClass,
                interfaceMethod.getName(),
                interfaceMethod.getParameterTypes());
        return implementationMethod != null
                && BridgeMethodResolver.findBridgedMethod(implementationMethod).equals(canonicalMethod);
    }

    private String buildInputSchema(Method method, Method contractMethod)
    {
        try
        {
            JsonNode schema = schemaGenerator.generate(method);
            JsonNode propertiesNode = schema.path("properties");
            if (propertiesNode instanceof ObjectNode propertiesObject)
            {
                Parameter[] parameters = method.getParameters();
                Parameter[] contractParameters = contractMethod.getParameters();
                for (int index = 0; index < parameters.length; index++)
                {
                    Parameter parameter = parameters[index];
                    Parameter contractParameter = contractParameters[index];
                    String schemaName = contractParameter.getName();
                    JsonNode parameterSchema = propertiesObject.remove(parameter.getName());
                    if (parameterSchema != null)
                    {
                        propertiesObject.set(schemaName, parameterSchema);
                        applyContractParameterMetadata((ObjectNode) schema, parameterSchema,
                                parameter.getName(), schemaName, contractParameter, parameter);
                        applyRuntimeInputSemantics(parameterSchema, objectMapper.constructType(parameter.getParameterizedType()));
                    }
                }
            }
            return objectMapper.writeValueAsString(schema);
        }
        catch (JacksonException ex)
        {
            throw new IllegalStateException("Failed to build method input schema for " + method, ex);
        }
    }

    private void applyContractParameterMetadata(ObjectNode rootSchema,
            JsonNode parameterSchema,
            String implementationParameterName,
            String parameterName,
            Parameter contractParameter,
            Parameter implementationParameter)
    {
        SkillParam skillParam = contractParameter.getAnnotation(SkillParam.class);
        if (skillParam == null)
        {
            skillParam = implementationParameter.getAnnotation(SkillParam.class);
        }
        if (skillParam != null
                && parameterSchema instanceof ObjectNode objectSchema
                && !skillParam.description().isBlank())
        {
            objectSchema.put("description", skillParam.description());
        }

        ArrayNode required = rootSchema.withArray("required");
        boolean generatedRequired = false;
        for (int index = required.size() - 1; index >= 0; index--)
        {
            String requiredName = required.get(index).asText();
            if (parameterName.equals(requiredName) || implementationParameterName.equals(requiredName))
            {
                generatedRequired = true;
                required.remove(index);
            }
        }
        if (skillParam == null ? generatedRequired : skillParam.required())
        {
            required.add(parameterName);
        }
    }

    private void applyRuntimeInputSemantics(JsonNode schemaNode, JavaType targetType)
    {
        if (!(schemaNode instanceof ObjectNode objectSchema) || targetType == null)
        {
            return;
        }

        Class<?> rawClass = targetType.getRawClass();
        if (isRefCapableBindableType(rawClass))
        {
            rewriteAsRefFriendlyString(objectSchema, rawClass);
            return;
        }

        if (targetType.isCollectionLikeType() || rawClass.isArray())
        {
            JsonNode itemsNode = objectSchema.get("items");
            if (itemsNode != null)
            {
                applyRuntimeInputSemantics(itemsNode, targetType.getContentType());
            }
            return;
        }

        if (targetType.isMapLikeType())
        {
            JsonNode additionalPropertiesNode = objectSchema.get("additionalProperties");
            if (additionalPropertiesNode != null && additionalPropertiesNode.isObject())
            {
                applyRuntimeInputSemantics(additionalPropertiesNode, targetType.getContentType());
            }
            return;
        }

        if (isSimpleBindableType(rawClass))
        {
            return;
        }

        JsonNode propertiesNode = objectSchema.get("properties");
        if (!(propertiesNode instanceof ObjectNode propertiesObject))
        {
            return;
        }

        propertyTypes(targetType).forEach((propertyName, propertyType) ->
        {
            JsonNode propertySchema = propertiesObject.get(propertyName);
            if (propertySchema != null)
            {
                applyRuntimeInputSemantics(propertySchema, propertyType);
            }
        });
    }

    private void rewriteAsRefFriendlyString(ObjectNode schemaNode, Class<?> rawClass)
    {
        schemaNode.removeAll();
        schemaNode.put("type", "string");
        schemaNode.put("description", refFriendlyDescription(rawClass));
        schemaNode.put("x-loomspan-runtime-ref-capable", true);
    }

    private String refFriendlyDescription(Class<?> rawClass)
    {
        if (byte[].class.equals(rawClass))
        {
            return "Provide a ref:// URI for binary content or an inline string value when appropriate.";
        }
        if (Resource.class.isAssignableFrom(rawClass))
        {
            return "Provide a ref:// URI for the resource content.";
        }
        if (InputStream.class.isAssignableFrom(rawClass))
        {
            return "Provide a ref:// URI for the stream content.";
        }
        return "Provide the value inline or as a ref:// URI.";
    }

    private Object invokeSkillMethod(String beanName,
            Method method,
            Method contractMethod,
            String targetId,
            Map<String, Object> arguments)
    {
        Map<String, Object> safeArguments = arguments == null ? Map.of() : arguments;
        ArrayList<InputStream> openedStreams = new ArrayList<>();

        try
        {
            if (beanFactory == null)
            {
                throw new IllegalStateException("BeanFactory has not been set for skill implementation target '" + targetId + "'");
            }
            Object bean = beanFactory.getBean(beanName);
            Method invocableMethod = selectRuntimeInvocableMethod(method, contractMethod, bean.getClass());
            Object[] invocationArguments = bindArguments(method, contractMethod, safeArguments, openedStreams);
            ReflectionUtils.makeAccessible(invocableMethod);
            Object result = ReflectionUtils.invokeMethod(invocableMethod, bean, invocationArguments);
            return objectMapper.writeValueAsString(result);
        }
        catch (JacksonException ex)
        {
            throw new IllegalStateException("Failed to serialize skill implementation target result for " + targetId, ex);
        }
        catch (IllegalArgumentException ex)
        {
            throw ex;
        }
        catch (RuntimeException ex)
        {
            log.warn("Skill implementation target '{}' failed during deterministic execution", targetId, ex);
            return LoomspanExceptionTransformer.transform(ex);
        }
        finally
        {
            closeStreams(targetId, openedStreams);
        }
    }

    private Method selectRuntimeInvocableMethod(Method canonicalMethod, Method contractMethod, Class<?> runtimeClass)
    {
        try
        {
            return AopUtils.selectInvocableMethod(canonicalMethod, runtimeClass);
        }
        catch (IllegalStateException ex)
        {
            if (contractMethod.getDeclaringClass().isInterface()
                    && contractMethod.getDeclaringClass().isAssignableFrom(runtimeClass))
            {
                return contractMethod;
            }

            List<Method> candidates = ClassUtils.getAllInterfacesForClassAsSet(runtimeClass).stream()
                    .flatMap(type -> java.util.Arrays.stream(type.getMethods()))
                    .filter(candidate -> mapsToCanonicalMethod(
                            candidate,
                            canonicalMethod.getDeclaringClass(),
                            canonicalMethod))
                    .sorted(java.util.Comparator.comparing(Method::toGenericString))
                    .toList();
            if (candidates.size() == 1)
            {
                return candidates.getFirst();
            }
            if (!candidates.isEmpty() && candidates.stream().allMatch(candidate ->
                    java.util.Arrays.equals(candidate.getParameterTypes(), candidates.getFirst().getParameterTypes())))
            {
                return candidates.getFirst();
            }
            throw ex;
        }
    }

    private Object[] bindArguments(Method method,
            Method contractMethod,
            Map<String, Object> arguments,
            List<InputStream> openedStreams)
    {
        Parameter[] parameters = method.getParameters();
        Parameter[] contractParameters = contractMethod.getParameters();
        Object[] bound = new Object[parameters.length];

        for (int index = 0; index < parameters.length; index++)
        {
            Parameter parameter = parameters[index];
            Parameter contractParameter = contractParameters[index];
            Object rawValue = arguments.get(contractParameter.getName());
            SkillParam skillParam = contractParameter.getAnnotation(SkillParam.class);
            if (skillParam == null)
            {
                skillParam = parameter.getAnnotation(SkillParam.class);
            }
            if (rawValue == null && skillParam != null && !skillParam.required())
            {
                bound[index] = null;
                continue;
            }
            bound[index] = convertArgument(parameter, rawValue, openedStreams);
        }
        return bound;
    }

    private Object convertArgument(Parameter parameter, Object rawValue, List<InputStream> openedStreams)
    {
        if (rawValue == null)
        {
            return null;
        }

        JavaType parameterJavaType = objectMapper.constructType(parameter.getParameterizedType());
        Class<?> parameterType = parameterJavaType.getRawClass();

        if (Resource.class.isAssignableFrom(parameterType))
        {
            return convertToResource(parameter, rawValue);
        }
        if (byte[].class.equals(parameterType))
        {
            return convertToBytes(parameter, rawValue);
        }
        if (InputStream.class.isAssignableFrom(parameterType))
        {
            InputStream stream = convertToInputStream(parameter, rawValue);
            openedStreams.add(stream);
            return stream;
        }
        if (String.class.equals(parameterType) && rawValue instanceof Resource resource)
        {
            return convertResourceToString(parameter, resource);
        }
        if (parameterType.isInstance(rawValue)
                && !parameterJavaType.isContainerType()
                && isSimpleBindableType(parameterType))
        {
            return rawValue;
        }

        Object materializedValue = materializeValue(rawValue, parameterJavaType, openedStreams);
        return objectMapper.convertValue(materializedValue, parameterJavaType);
    }

    private Object materializeValue(Object rawValue, JavaType targetType, List<InputStream> openedStreams)
    {
        if (rawValue == null)
        {
            return null;
        }

        Class<?> rawClass = targetType.getRawClass();
        if (Resource.class.isAssignableFrom(rawClass))
        {
            return rawValue;
        }
        if (String.class.equals(rawClass) && rawValue instanceof Resource resource)
        {
            return convertResourceToString(null, resource);
        }
        if (byte[].class.equals(rawClass) && rawValue instanceof Resource resource)
        {
            return convertToBytes(null, resource);
        }
        if (InputStream.class.isAssignableFrom(rawClass) && rawValue instanceof Resource resource)
        {
            InputStream stream = convertToInputStream(null, resource);
            openedStreams.add(stream);
            return stream;
        }
        if (targetType.isMapLikeType() && rawValue instanceof Map<?, ?> mapValue)
        {
            JavaType valueType = targetType.getContentType() == null
                    ? objectMapper.constructType(Object.class)
                    : targetType.getContentType();
            Map<Object, Object> materialized = new LinkedHashMap<>();
            mapValue.forEach((key, value) -> materialized.put(key, materializeValue(value, valueType, openedStreams)));
            return materialized;
        }
        if (targetType.isCollectionLikeType() && rawValue instanceof List<?> listValue)
        {
            JavaType contentType = targetType.getContentType() == null
                    ? objectMapper.constructType(Object.class)
                    : targetType.getContentType();
            return listValue.stream()
                    .map(value -> materializeValue(value, contentType, openedStreams))
                    .toList();
        }
        if (rawClass.isArray() && rawValue instanceof List<?> listValue)
        {
            JavaType contentType = targetType.getContentType() == null
                    ? objectMapper.constructType(Object.class)
                    : targetType.getContentType();
            return listValue.stream()
                    .map(value -> materializeValue(value, contentType, openedStreams))
                    .toList();
        }
        if (rawValue instanceof Map<?, ?> mapValue && !isSimpleBindableType(rawClass))
        {
            Map<String, JavaType> propertyTypes = propertyTypes(targetType);
            Map<String, Object> materialized = new LinkedHashMap<>();
            mapValue.forEach((key, value) ->
            {
                String propertyName = String.valueOf(key);
                JavaType propertyType = propertyTypes.getOrDefault(propertyName, objectMapper.constructType(Object.class));
                materialized.put(propertyName, materializeValue(value, propertyType, openedStreams));
            });
            return materialized;
        }
        return rawValue;
    }

    private Map<String, JavaType> propertyTypes(JavaType targetType)
    {
        return objectMapper._deserializationContext()
                .introspectBeanDescriptionForCreation(targetType)
                .findProperties()
                .stream()
                .filter(definition -> definition.getPrimaryMember() != null)
                .collect(Collectors.toMap(
                        BeanPropertyDefinition::getName,
                        definition -> definition.getPrimaryMember().getType(),
                        (left, right) -> left,
                        LinkedHashMap::new));
    }

    private boolean isSimpleBindableType(Class<?> rawClass)
    {
        return rawClass.isPrimitive()
                || Number.class.isAssignableFrom(rawClass)
                || CharSequence.class.isAssignableFrom(rawClass)
                || Boolean.class.equals(rawClass)
                || Enum.class.isAssignableFrom(rawClass)
                || Object.class.equals(rawClass);
    }

    private boolean isRefCapableBindableType(Class<?> rawClass)
    {
        return byte[].class.equals(rawClass)
                || Resource.class.isAssignableFrom(rawClass)
                || InputStream.class.isAssignableFrom(rawClass);
    }

    private Resource convertToResource(Parameter parameter, Object rawValue)
    {
        if (rawValue instanceof Resource resource)
        {
            return resource;
        }
        throw new IllegalArgumentException("Parameter '" + parameterName(parameter) + "' requires a Resource-backed ref payload");
    }

    private byte[] convertToBytes(Parameter parameter, Object rawValue)
    {
        if (rawValue instanceof byte[] bytes)
        {
            return bytes;
        }
        if (rawValue instanceof Resource resource)
        {
            try
            {
                return StreamUtils.copyToByteArray(resource.getInputStream());
            }
            catch (IOException ex)
            {
                throw new IllegalStateException("Failed to read binary payload for parameter '" + parameterName(parameter) + "'", ex);
            }
        }
        return objectMapper.convertValue(rawValue, byte[].class);
    }

    private InputStream convertToInputStream(Parameter parameter, Object rawValue)
    {
        if (rawValue instanceof InputStream stream)
        {
            return stream;
        }
        if (rawValue instanceof Resource resource)
        {
            try
            {
                return resource.getInputStream();
            }
            catch (IOException ex)
            {
                throw new IllegalStateException("Failed to open stream for parameter '" + parameterName(parameter) + "'", ex);
            }
        }
        throw new IllegalArgumentException("Parameter '" + parameterName(parameter) + "' requires a Resource-backed ref payload");
    }

    private String convertResourceToString(Parameter parameter, Resource resource)
    {
        try
        {
            return StreamUtils.copyToString(resource.getInputStream(), StandardCharsets.UTF_8);
        }
        catch (IOException ex)
        {
            throw new IllegalStateException("Failed to read text payload for parameter '" + parameterName(parameter) + "'", ex);
        }
    }

    private String parameterName(Parameter parameter)
    {
        return parameter == null ? "<nested>" : parameter.getName();
    }

    private void closeStreams(String capabilityName, List<InputStream> openedStreams)
    {
        for (InputStream stream : openedStreams)
        {
            try
            {
                stream.close();
            }
            catch (IOException ex)
            {
                log.warn("Capability '{}' failed while closing opened ref stream", capabilityName, ex);
            }
        }
    }
}
